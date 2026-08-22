import XCTest
@testable import CodexTweaks

final class CDPTargetSelectorTests: XCTestCase {
    func testSelectsOnlyInjectableAppPages() throws {
        let data = Data(
            """
            [
              {
                "id": "codex-main",
                "type": "page",
                "title": "Codex",
                "url": "app://-/index.html",
                "webSocketDebuggerUrl": "ws://127.0.0.1:9335/devtools/page/codex-main"
              },
              {
                "id": "browser",
                "type": "page",
                "title": "Example",
                "url": "https://example.com",
                "webSocketDebuggerUrl": "ws://127.0.0.1:9335/devtools/page/browser"
              },
              {
                "id": "worker",
                "type": "service_worker",
                "title": "Worker",
                "url": "app://-/worker.js",
                "webSocketDebuggerUrl": "ws://127.0.0.1:9335/devtools/page/worker"
              }
            ]
            """.utf8
        )

        let targets = try CDPTargetSelector.injectableTargets(from: data)

        XCTAssertEqual(targets.map(\.id), ["codex-main"])
    }

    func testRejectsAppPageWithoutDebuggerURL() throws {
        let data = Data(
            """
            [{"id":"page","type":"page","title":"Codex","url":"app://-/index.html"}]
            """.utf8
        )

        XCTAssertTrue(try CDPTargetSelector.injectableTargets(from: data).isEmpty)
    }
}
