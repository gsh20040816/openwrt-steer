// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation
import XCTest
@testable import SteerApp

final class SubscriptionStatusTests: XCTestCase {
    private struct FixtureDocument: Decodable {
        let schemaVersion: Int
        let cases: [FixtureCase]

        enum CodingKeys: String, CodingKey {
            case schemaVersion = "schema_version"
            case cases
        }
    }

    private struct FixtureCase: Decodable {
        let name: String
        let status: SubscriptionRuntimeStatus
    }

    private func fixtures() throws -> FixtureDocument {
        let repositoryRoot = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        let url = repositoryRoot.appendingPathComponent("ui/subscription-status-fixtures.json")
        let decoder = JSONDecoder()
        return try decoder.decode(FixtureDocument.self, from: Data(contentsOf: url))
    }

    func testSharedSubscriptionStatusFixtureCoversEveryLifecycleState() throws {
        let document = try fixtures()
        XCTAssertEqual(document.schemaVersion, 1)
        XCTAssertEqual(Set(document.cases.map(\.name)), Set([
            "never-fetched", "success", "success-with-skipped",
            "failed-after-success-with-partial-stale-block", "disabled",
        ]))

        let byName = Dictionary(uniqueKeysWithValues: document.cases.map { ($0.name, $0.status) })
        XCTAssertEqual(byName["never-fetched"]?.stateLabel, "未抓取")
        XCTAssertEqual(byName["success"]?.stateLabel, "成功")
        XCTAssertEqual(byName["success-with-skipped"]?.stateLabel, "成功 · 跳过 2")
        XCTAssertEqual(byName["disabled"]?.stateLabel, "已停用")

        let failed = try XCTUnwrap(byName["failed-after-success-with-partial-stale-block"])
        XCTAssertEqual(failed.stateLabel, "最近失败")
        XCTAssertEqual(failed.lastSuccess, "2026-08-26T02:00:00Z")
        XCTAssertEqual(failed.lastFailure?.summary, "subscription server returned HTTP 503")
        XCTAssertEqual(failed.inventorySummary, "added 0 · current 1 · stale 2 · skipped 1")
        XCTAssertEqual(failed.stale[0].referencedBy.first?.id, "proxy")
        XCTAssertTrue(failed.stale[1].referencedBy.isEmpty)
    }
}
