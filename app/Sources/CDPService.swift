import Foundation

enum CDPServiceError: LocalizedError {
    case endpointUnavailable
    case invalidResponse
    case commandRejected(String)
    case malformedMessage

    var errorDescription: String? {
        switch self {
        case .endpointUnavailable:
            return "Codex 未开启本地 CDP 端口"
        case .invalidResponse:
            return "CDP 返回了无效响应"
        case let .commandRejected(message):
            return "CDP 拒绝执行：\(message)"
        case .malformedMessage:
            return "CDP 返回了无法解析的消息"
        }
    }
}
actor CDPService {
    static let endpoint = URL(string: "http://127.0.0.1:9335/json/list")!
    static let allowedOrigin = "http://127.0.0.1:9335"

    private var nextCommandID = 1
    private let session: URLSession

    init() {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = 3
        configuration.timeoutIntervalForResource = 5
        session = URLSession(configuration: configuration)
    }

    func inject(payload: TweakPayload, forceGeneration: Int) async throws -> CDPInjectionResult {
        let targets = try await discoverTargets()
        let script = InjectionScriptBuilder.injectionScript(
            payload: payload,
            forceGeneration: forceGeneration
        )
        var successCount = 0

        for target in targets {
            guard let url = target.debuggerURL else { continue }
            do {
                try await evaluate(script, in: url)
                successCount += 1
            } catch {
                AppLogger.shared.error("目标 \(target.id) 注入失败：\(error)")
            }
        }

        return CDPInjectionResult(targetCount: targets.count, successCount: successCount)
    }

    func cleanupAllTargets() async throws {
        let targets = try await discoverTargets()
        for target in targets {
            guard let url = target.debuggerURL else { continue }
            do {
                try await evaluate(InjectionScriptBuilder.cleanupScript, in: url)
            } catch {
                AppLogger.shared.error("目标 \(target.id) 清理失败：\(error)")
            }
        }
    }

    private func discoverTargets() async throws -> [CDPTarget] {
        do {
            let (data, response) = try await session.data(from: Self.endpoint)
            guard let httpResponse = response as? HTTPURLResponse,
                  (200 ... 299).contains(httpResponse.statusCode)
            else {
                throw CDPServiceError.invalidResponse
            }
            return try CDPTargetSelector.injectableTargets(from: data)
        } catch let error as CDPServiceError {
            throw error
        } catch let error as URLError {
            switch error.code {
            case .cannotConnectToHost, .cannotFindHost, .networkConnectionLost, .timedOut:
                throw CDPServiceError.endpointUnavailable
            default:
                throw error
            }
        }
    }

    private func evaluate(_ expression: String, in debuggerURL: URL) async throws {
        var request = URLRequest(url: debuggerURL)
        request.timeoutInterval = 5
        request.setValue(Self.allowedOrigin, forHTTPHeaderField: "Origin")

        let socket = session.webSocketTask(with: request)
        socket.resume()
        defer {
            socket.cancel(with: .normalClosure, reason: nil)
        }

        let commandID = nextCommandID
        nextCommandID &+= 1

        let command: [String: Any] = [
            "id": commandID,
            "method": "Runtime.evaluate",
            "params": [
                "expression": expression,
                "returnByValue": true,
                "awaitPromise": true,
                "userGesture": false,
            ],
        ]
        let commandData = try JSONSerialization.data(withJSONObject: command)
        guard let commandText = String(data: commandData, encoding: .utf8) else {
            throw CDPServiceError.malformedMessage
        }

        try await socket.send(.string(commandText))

        while true {
            let message = try await socket.receive()
            let data: Data
            switch message {
            case let .data(value):
                data = value
            case let .string(value):
                guard let valueData = value.data(using: .utf8) else {
                    throw CDPServiceError.malformedMessage
                }
                data = valueData
            @unknown default:
                throw CDPServiceError.malformedMessage
            }

            guard let object = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
                throw CDPServiceError.malformedMessage
            }
            guard let responseID = object["id"] as? Int, responseID == commandID else {
                continue
            }
            if let error = object["error"] as? [String: Any] {
                throw CDPServiceError.commandRejected(error["message"] as? String ?? "未知错误")
            }
            if let result = object["result"] as? [String: Any],
               let exception = result["exceptionDetails"] as? [String: Any]
            {
                let description = (exception["exception"] as? [String: Any])?["description"] as? String
                    ?? exception["text"] as? String
                    ?? "注入脚本执行失败"
                throw CDPServiceError.commandRejected(description)
            }
            return
        }
    }
}
