// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation
import XCTest
@testable import SteerApp

final class StateLifecycleTests: XCTestCase {
    private struct FixtureDocument: Decodable {
        let schemaVersion: Int
        let cases: [Fixture]

        enum CodingKeys: String, CodingKey {
            case schemaVersion = "schema_version"
            case cases
        }
    }

    private struct Fixture: Decodable {
        let name: String
        let draft: Draft
        let saved: Saved
        let active: Active
        let pendingApply: Bool

        enum CodingKeys: String, CodingKey {
            case name, draft, saved, active
            case pendingApply = "pending_apply"
        }
    }

    private struct Draft: Decodable { let dirty: Bool; let enabled: Bool }
    private struct Saved: Decodable { let enabled: Bool; let revision: String }
    private struct Active: Decodable {
        let running: Bool
        let healthy: Bool
        let generation: String?
        let digest: String?
        let lastApplyOK: Bool?

        enum CodingKeys: String, CodingKey {
            case running, healthy, generation, digest
            case lastApplyOK = "last_apply_ok"
        }
    }

    func testSharedStateLifecycleFixtureKeepsDraftSavedAndActiveIndependent() throws {
        let repositoryRoot = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        let data = try Data(contentsOf: repositoryRoot.appendingPathComponent("ui/state-lifecycle-fixtures.json"))
        let document = try JSONDecoder().decode(FixtureDocument.self, from: data)
        XCTAssertEqual(document.schemaVersion, 1)
        XCTAssertEqual(document.cases.map(\.name), ["fresh", "pending-disable", "failed-apply", "active"])

        let pendingDisable = try XCTUnwrap(document.cases.first { $0.name == "pending-disable" })
        XCTAssertTrue(pendingDisable.draft.dirty)
        XCTAssertFalse(pendingDisable.draft.enabled)
        XCTAssertTrue(pendingDisable.saved.enabled)
        XCTAssertTrue(pendingDisable.active.running)
        XCTAssertTrue(pendingDisable.active.healthy)
        XCTAssertEqual(pendingDisable.active.generation, "generation-old")

        let failedApply = try XCTUnwrap(document.cases.first { $0.name == "failed-apply" })
        XCTAssertTrue(failedApply.pendingApply)
        XCTAssertEqual(failedApply.saved.revision, "saved-new")
        XCTAssertEqual(failedApply.active.generation, "generation-old")
        XCTAssertEqual(failedApply.active.lastApplyOK, false)
    }
}
