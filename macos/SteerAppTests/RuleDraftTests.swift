// SPDX-License-Identifier: GPL-3.0-or-later

import XCTest
import SwiftUI
@testable import SteerApp

final class DraftStringListCodecTests: XCTestCase {
    func testCommaBearingEntriesRoundTripWithoutRewriting() {
        let values = [
            "regexp:^api[0-9]{1,3}\\.example\\.com$",
            "regexp:^[a,b]+$",
            " geosite:category-example@test ",
            "--SocksPort,19050",
        ]

        XCTAssertEqual(DraftStringListCodec.values(from: DraftStringListCodec.text(from: values)), values)
    }

    func testBlankLinesAreFilteredButEntryBodiesArePreserved() {
        let text = "first\n\n   \n second,entry \n\tthird\t"

        XCTAssertEqual(
            DraftStringListCodec.values(from: text),
            ["first", " second,entry ", "\tthird\t"]
        )
    }

    func testStringListBindingLoadsAndSavesTheSameArray() {
        let values = [
            "regexp:^api[0-9]{1,3}\\.example\\.com$",
            " regexp:^[a,b]+$ ",
            "geosite:category-media@test",
        ]
        var object: [String: JSONValue] = [
            "domain_match": .array(values.map(JSONValue.string)),
        ]
        let objectBinding = Binding<[String: JSONValue]>(
            get: { object },
            set: { object = $0 }
        )
        let editorBinding = stringListBinding(objectBinding, "domain_match")

        XCTAssertEqual(editorBinding.wrappedValue, values.joined(separator: "\n"))
        editorBinding.wrappedValue += "\n\n   "
        XCTAssertEqual(
            object["domain_match"]?.arrayValue?.compactMap(\.stringValue),
            values
        )
    }

    func testRepresentativeStringListFieldsKeepCommasInsideTokens() {
        let samples = [
            "domain_match": "regexp:^api[0-9]{1,3}\\.example\\.com$",
            "ip_match": "regexp:^[0-9a-f]{1,4},[0-9a-f]{1,4}$",
            "source_ip_cidr": "192.168.1.0/24",
            "server_ports": "20000:21000,22000:23000",
            "host_key_algorithms": "ssh-ed25519,rsa-sha2-512",
            "extra_args": "--SocksPort,19050",
        ]

        for (field, value) in samples {
            XCTAssertEqual(DraftStringListCodec.values(from: value), [value], field)
        }
    }

    func testGeoCompletionAppendsWithoutChangingExistingSource() {
        let original = " regexp:^api[0-9]{1,3}\\.example\\.com$ \ngeosite:cn"
        let completed = DraftStringListCodec.appendingUnique("geosite:category-media@test", to: original)

        XCTAssertEqual(
            DraftStringListCodec.values(from: completed),
            [
                " regexp:^api[0-9]{1,3}\\.example\\.com$ ",
                "geosite:cn",
                "geosite:category-media@test",
            ]
        )
        XCTAssertEqual(
            DraftStringListCodec.appendingUnique("geosite:cn", to: completed),
            completed
        )
    }
}

@MainActor
final class AppModelRulePolicyTests: XCTestCase {
    func testDefaultCannotBeDisabledDeletedOrMoved() {
        let model = makeModel()

        model.setDraftItemEnabled(in: "rules", at: 1, enabled: false)
        model.moveDraftItem(in: "rules", at: 1, offset: -1)
        model.moveDraftItem(in: "rules", at: 0, offset: 1)
        model.moveDraftItem(in: "rules", from: IndexSet(integer: 1), to: 0)
        model.removeDraftItem(from: "rules", at: 1)

        let rules = ruleObjects(model)
        XCTAssertEqual(rules.compactMap { $0["id"]?.stringValue }, ["ordinary", "default"])
        XCTAssertEqual(rules.last?["enabled"]?.boolValue, true)
        XCTAssertEqual(rules.last?["default"]?.boolValue, true)
        XCTAssertEqual(model.message, "Default 规则必须保留")
    }

    func testDefaultEditorOnlyChangesNameAndDecision() {
        let model = makeModel()
        var proposed = ruleObjects(model)[1]
        proposed["id"] = .string("replacement")
        proposed["enabled"] = .bool(false)
        proposed["default"] = .bool(false)
        proposed["name"] = .string("Fallback")
        proposed["dns_profile"] = .string("dns_alt")
        proposed["route"] = .string("proxy")
        proposed["domain_match"] = .array([.string("domain:should-not-survive.example")])

        XCTAssertTrue(model.replaceDraftItem(in: "rules", at: 1, object: proposed))

        let pinned = ruleObjects(model).last
        XCTAssertEqual(pinned?["id"]?.stringValue, "default")
        XCTAssertEqual(pinned?["enabled"]?.boolValue, true)
        XCTAssertEqual(pinned?["default"]?.boolValue, true)
        XCTAssertEqual(pinned?["name"]?.stringValue, "Fallback")
        XCTAssertEqual(pinned?["dns_profile"]?.stringValue, "dns_alt")
        XCTAssertEqual(pinned?["route"]?.stringValue, "proxy")
        XCTAssertNil(pinned?["domain_match"])
    }

    func testOrdinaryRuleCannotBecomeASecondDefault() {
        let model = makeModel()
        var proposed = ruleObjects(model)[0]
        proposed["default"] = .bool(true)

        XCTAssertTrue(model.replaceDraftItem(in: "rules", at: 0, object: proposed))
        XCTAssertEqual(ruleObjects(model).filter(RuleDraftPolicy.isDefault).count, 1)
        XCTAssertEqual(ruleObjects(model)[0]["default"]?.boolValue, false)

        XCTAssertFalse(model.appendDraftItem(to: "rules", object: proposed))
        XCTAssertEqual(ruleObjects(model).filter(RuleDraftPolicy.isDefault).count, 1)
    }

    func testNewOrdinaryRuleIsInsertedBeforeDefault() {
        let model = makeModel()
        let newRule: [String: JSONValue] = [
            "id": .string("second"),
            "enabled": .bool(true),
            "default": .bool(false),
            "dns_profile": .string("dns"),
            "route": .string("direct"),
            "domain_match": .array([.string("domain:second.example")]),
        ]

        XCTAssertTrue(model.appendDraftItem(to: "rules", object: newRule))
        XCTAssertEqual(
            ruleObjects(model).compactMap { $0["id"]?.stringValue },
            ["ordinary", "second", "default"]
        )
    }

    func testStableIdentifierToggleResolvesTheCurrentRuleAfterReordering() {
        let model = makeModel()
        let second: [String: JSONValue] = [
            "id": .string("second"),
            "enabled": .bool(true),
            "default": .bool(false),
            "dns_profile": .string("dns"),
            "route": .string("direct"),
            "domain_match": .array([.string("domain:second.example")]),
        ]
        XCTAssertTrue(model.appendDraftItem(to: "rules", object: second))
        model.moveDraftItem(in: "rules", at: 0, offset: 1)

        model.setDraftItemEnabled(in: "rules", identifiedBy: "ordinary", enabled: false)

        XCTAssertEqual(model.draftItemEnabled(in: "rules", identifiedBy: "ordinary"), false)
        XCTAssertEqual(model.draftItemEnabled(in: "rules", identifiedBy: "second"), true)
        model.setDraftItemEnabled(in: "rules", identifiedBy: "default", enabled: false)
        XCTAssertEqual(model.draftItemEnabled(in: "rules", identifiedBy: "default"), true)
    }

    private func makeModel() -> AppModel {
        let model = AppModel()
        model.rawJSON = #"""
        {
          "main": {"schema_version": 9, "enabled": false},
          "routes": [
            {"id": "direct", "enabled": true, "kind": "direct"},
            {"id": "proxy", "enabled": true, "kind": "single", "node": "node"}
          ],
          "dns_profiles": [
            {"id": "dns", "enabled": true, "protocol": "udp", "server": "1.1.1.1", "server_port": 53},
            {"id": "dns_alt", "enabled": true, "protocol": "udp", "server": "8.8.8.8", "server_port": 53}
          ],
          "rules": [
            {
              "id": "ordinary", "enabled": true, "name": "Ordinary", "default": false,
              "dns_profile": "dns", "route": "direct", "domain_match": ["domain:example.com"]
            },
            {
              "id": "default", "enabled": true, "name": "Default", "default": true,
              "dns_profile": "dns", "route": "direct"
            }
          ]
        }
        """#
        model.isDirty = false
        return model
    }

    private func ruleObjects(_ model: AppModel) -> [[String: JSONValue]] {
        model.draftItems(for: "rules").compactMap {
            model.draftItemObject(for: "rules", at: $0.index)
        }
    }
}
