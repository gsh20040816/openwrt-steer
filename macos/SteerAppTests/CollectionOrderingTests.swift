// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation
import XCTest
@testable import SteerApp

@MainActor
final class CollectionOrderingTests: XCTestCase {
    private struct Document: Decodable {
        let schemaVersion: Int
        let collections: [String]
        let cases: [OrderingCase]

        enum CodingKeys: String, CodingKey {
            case schemaVersion = "schema_version"
            case collections, cases
        }
    }

    private struct OrderingCase: Decodable {
        let name: String
        let collection: String
        let objects: [JSONValue]
        let visibleIDs: [String]
        let moveID: String
        let offset: Int
        let expectedIDs: [String]

        enum CodingKeys: String, CodingKey {
            case name, collection, objects, offset
            case visibleIDs = "visible_ids"
            case moveID = "move_id"
            case expectedIDs = "expected_ids"
        }
    }

    private var repositoryRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }

    private func fixture() throws -> Document {
        let data = try Data(contentsOf: repositoryRoot.appendingPathComponent("ui/collection-ordering-fixtures.json"))
        return try JSONDecoder().decode(Document.self, from: data)
    }

    func testSharedCollectionOrderingPoliciesDecode() throws {
        let document = try fixture()
        XCTAssertEqual(document.schemaVersion, 1)
        XCTAssertEqual(Set(document.collections), Set(SteerUISpec.contract.collectionOrdering.keys))
        XCTAssertEqual(SteerUISpec.orderingPolicy(for: "nodes")?.groupField, "source_subscription")
        XCTAssertEqual(SteerUISpec.orderingPolicy(for: "routes")?.movableKinds, ["single"])
        XCTAssertEqual(SteerUISpec.orderingPolicy(for: "rules")?.pinnedLastBooleanField, "default")
    }

    func testEveryCollectionMovesByStableIDWithoutChangingObjectIdentity() throws {
        for testCase in try fixture().cases {
            let model = AppModel()
            var root: [String: JSONValue] = [
                "main": .object(["schema_version": .number(9), "enabled": .bool(false)]),
                "bootstrap": .object(["protocol": .string("udp"), "server": .string("1.1.1.1"), "server_port": .number(53)]),
            ]
            root[testCase.collection] = .array(testCase.objects)
            let data = try JSONEncoder().encode(JSONValue.object(root))
            model.rawJSON = String(decoding: data, as: UTF8.self)
            model.isDirty = false

            XCTAssertTrue(model.moveDraftItem(
                in: testCase.collection,
                identifiedBy: testCase.moveID,
                offset: testCase.offset,
                visibleIDs: testCase.visibleIDs
            ), testCase.name)
            XCTAssertEqual(model.draftItems(for: testCase.collection).map(\.identifier), testCase.expectedIDs, testCase.name)
            XCTAssertTrue(model.isDirty, testCase.name)
            XCTAssertEqual(
                Set(model.draftItems(for: testCase.collection).map(\.identifier)),
                Set(testCase.objects.compactMap { $0.objectValue?["id"]?.stringValue }),
                testCase.name
            )
        }
    }

    func testPinnedAndBoundaryMovesDoNotDirtyTheDraft() throws {
        let testCase = try XCTUnwrap(try fixture().cases.first(where: { $0.collection == "rules" }))
        let model = AppModel()
        let root = JSONValue.object([
            "main": .object(["schema_version": .number(9), "enabled": .bool(false)]),
            "bootstrap": .object(["protocol": .string("udp"), "server": .string("1.1.1.1"), "server_port": .number(53)]),
            "rules": .array(testCase.objects),
        ])
        model.rawJSON = String(decoding: try JSONEncoder().encode(root), as: UTF8.self)
        model.isDirty = false

        XCTAssertFalse(model.moveDraftItem(in: "rules", identifiedBy: "default", offset: -1, visibleIDs: testCase.visibleIDs))
        XCTAssertFalse(model.moveDraftItem(in: "rules", identifiedBy: "rule_a", offset: -1, visibleIDs: testCase.visibleIDs))
        XCTAssertFalse(model.isDirty)
    }
}
