import Foundation

enum BackendClientError: LocalizedError {
    case executableNotFound
    case notRunning
    case terminated(Int32)
    case malformedResponse
    case remote(String)

    var errorDescription: String? {
        switch self {
        case .executableNotFound:
            return PresentationText.resolve(.appBackendMissing)
        case .notRunning:
            return PresentationText.resolve(.appBackendNotRunning)
        case let .terminated(status):
            return PresentationText.resolve(
                .appBackendTerminated,
                replacements: ["status": String(status)]
            )
        case .malformedResponse:
            return PresentationText.resolve(.appBackendMalformed)
        case let .remote(message):
            return message
        }
    }
}

final class BackendClient: @unchecked Sendable {
    static let shared = BackendClient()

    var stateHandler: (@MainActor @Sendable (BackendAppSnapshot) -> Void)? {
        get { queue.sync { storedStateHandler } }
        set { queue.sync { storedStateHandler = newValue } }
    }

    private let queue = DispatchQueue(label: "com.zgccrui.CodexTweaks.backend")
    private var storedStateHandler: (@MainActor @Sendable (BackendAppSnapshot) -> Void)?
    private var process: Process?
    private var inputHandle: FileHandle?
    private var outputHandle: FileHandle?
    private var outputBuffer = Data()
    private var nextRequestID: Int64 = 1
    private var pending: [Int64: @Sendable (Data) -> Void] = [:]

    private init() {}

    func start(params: BackendInitializeParams) async throws -> BackendAppSnapshot {
        try launchIfNeeded()
        return try await request(method: "initialize", params: params)
    }

    func send<Params: Encodable & Sendable>(
        method: String,
        params: Params
    ) async throws {
        let _: BackendAccepted = try await request(method: method, params: params)
    }

    func send(method: String) async throws {
        try await send(method: method, params: EmptyParams())
    }

    func request<Result: Decodable & Sendable, Params: Encodable & Sendable>(
        method: String,
        params: Params
    ) async throws -> Result {
        try await withCheckedThrowingContinuation { continuation in
            queue.async { [weak self] in
                guard let self, let inputHandle, process?.isRunning == true else {
                    continuation.resume(throwing: BackendClientError.notRunning)
                    return
                }
                let requestID = nextRequestID
                nextRequestID &+= 1
                do {
                    let envelope = BackendRequest(id: requestID, method: method, params: params)
                    var data = try Self.makeEncoder().encode(envelope)
                    data.append(0x0A)
                    pending[requestID] = { responseData in
                        do {
                            let response = try Self.makeDecoder().decode(
                                BackendResponse<Result>.self,
                                from: responseData
                            )
                            if let error = response.error {
                                continuation.resume(throwing: BackendClientError.remote(error.message))
                            } else if let result = response.result {
                                continuation.resume(returning: result)
                            } else {
                                continuation.resume(throwing: BackendClientError.malformedResponse)
                            }
                        } catch {
                            continuation.resume(throwing: error)
                        }
                    }
                    do {
                        try inputHandle.write(contentsOf: data)
                    } catch {
                        pending.removeValue(forKey: requestID)
                        continuation.resume(throwing: error)
                    }
                } catch {
                    continuation.resume(throwing: error)
                }
            }
        }
    }

    func stop() async {
        try? await send(method: "shutdown")
        queue.sync { [weak self] in
            self?.closeProcess()
        }
    }

    private func launchIfNeeded() throws {
        try queue.sync {
            guard process?.isRunning != true else { return }
            guard let executableURL = Self.backendExecutableURL() else {
                throw BackendClientError.executableNotFound
            }

            let process = Process()
            let input = Pipe()
            let output = Pipe()
            process.executableURL = executableURL
            process.standardInput = input
            process.standardOutput = output
            process.standardError = FileHandle.standardError
            process.terminationHandler = { [weak self] process in
                self?.queue.async {
                    self?.handleTermination(status: process.terminationStatus)
                }
            }

            try process.run()
            self.process = process
            inputHandle = input.fileHandleForWriting
            outputHandle = output.fileHandleForReading
            output.fileHandleForReading.readabilityHandler = { [weak self] handle in
                let data = handle.availableData
                self?.queue.async {
                    self?.consumeOutput(data)
                }
            }
        }
    }

    private func consumeOutput(_ data: Data) {
        guard !data.isEmpty else {
            outputHandle?.readabilityHandler = nil
            return
        }
        outputBuffer.append(data)
        while let newline = outputBuffer.firstIndex(of: 0x0A) {
            let line = Data(outputBuffer[..<newline])
            outputBuffer.removeSubrange(...newline)
            guard !line.isEmpty else { continue }
            handleLine(line)
        }
    }

    private func handleLine(_ data: Data) {
        guard let header = try? Self.makeDecoder().decode(BackendMessageHeader.self, from: data) else {
            return
        }
        if header.event == "state",
           let message = try? Self.makeDecoder().decode(BackendStateEvent.self, from: data) {
            let handler = storedStateHandler
            Task { @MainActor in
                handler?(message.data)
            }
            return
        }
        if let id = header.id, let completion = pending.removeValue(forKey: id) {
            completion(data)
        }
    }

    private func handleTermination(status: Int32) {
        let callbacks = Array(pending.values)
        pending.removeAll()
        let data = (try? JSONSerialization.data(withJSONObject: [
            "id": 0,
            "error": [
                "code": "terminated",
                "message": BackendClientError.terminated(status).localizedDescription,
            ],
        ])) ?? Data()
        callbacks.forEach { $0(data) }
        closeProcess()
    }

    private func closeProcess() {
        outputHandle?.readabilityHandler = nil
        try? inputHandle?.close()
        try? outputHandle?.close()
        inputHandle = nil
        outputHandle = nil
        process = nil
        outputBuffer.removeAll(keepingCapacity: false)
    }

    private static func backendExecutableURL() -> URL? {
        if let override = ProcessInfo.processInfo.environment["CODEX_TWEAKS_BACKEND_PATH"],
           FileManager.default.isExecutableFile(atPath: override) {
            return URL(fileURLWithPath: override)
        }
        return Bundle.main.url(forResource: "codex-tweaks-backend", withExtension: nil)
    }

    static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let value = try container.decode(String.self)
            let formatter = ISO8601DateFormatter()
            formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let date = formatter.date(from: value) { return date }
            formatter.formatOptions = [.withInternetDateTime]
            if let date = formatter.date(from: value) { return date }
            throw DecodingError.dataCorruptedError(
                in: container,
                debugDescription: PresentationText.resolve(
                    .appBackendDateMalformed,
                    replacements: ["value": value]
                )
            )
        }
        return decoder
    }

    static func makeEncoder() -> JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return encoder
    }
}

private struct EmptyParams: Encodable, Sendable {}

private struct BackendRequest<Params: Encodable>: Encodable {
    let id: Int64
    let method: String
    let params: Params
}

private struct BackendMessageHeader: Decodable {
    let id: Int64?
    let event: String?
}

private struct BackendStateEvent: Decodable {
    let event: String
    let data: BackendAppSnapshot
}

private struct BackendErrorPayload: Decodable {
    let code: String
    let message: String
}

private struct BackendResponse<Result: Decodable>: Decodable {
    let id: Int64
    let result: Result?
    let error: BackendErrorPayload?
}
