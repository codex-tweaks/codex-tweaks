import Foundation
import XCTest

final class MacOSAgentModePreferencesTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suiteName: String!

    override func setUp() {
        super.setUp()
        suiteName = "MacOSAgentModePreferencesTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suiteName)
        defaults = nil
        suiteName = nil
        super.tearDown()
    }

    func testDefaultsKeepNormalModeVisibleAndHideAgentMenuBar() {
        XCTAssertFalse(MacOSAgentModePreferences.isEnabled(in: defaults))
        XCTAssertTrue(MacOSAgentModePreferences.hidesMenuBarIcon(in: defaults))
        XCTAssertTrue(
            MacOSAgentModePreferences.shouldShowMenuBarIcon(
                isEnabled: false,
                hidesMenuBarIcon: true
            )
        )
    }

    func testAgentModePreferencePersistsAcrossReads() {
        MacOSAgentModePreferences.setEnabled(true, in: defaults)
        MacOSAgentModePreferences.setHidesMenuBarIcon(false, in: defaults)

        XCTAssertTrue(MacOSAgentModePreferences.isEnabled(in: defaults))
        XCTAssertFalse(MacOSAgentModePreferences.hidesMenuBarIcon(in: defaults))
    }

    func testAgentModeCanRemoveOrRetainTheMenuBarIcon() {
        XCTAssertFalse(
            MacOSAgentModePreferences.shouldShowMenuBarIcon(
                isEnabled: true,
                hidesMenuBarIcon: true
            )
        )
        XCTAssertTrue(
            MacOSAgentModePreferences.shouldShowMenuBarIcon(
                isEnabled: true,
                hidesMenuBarIcon: false
            )
        )
    }
}
