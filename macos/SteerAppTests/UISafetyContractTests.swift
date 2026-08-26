// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation
import XCTest
@testable import SteerApp

final class UISafetyContractTests: XCTestCase {
    private struct ReferenceDocument: Decodable {
        let schemaVersion: Int
        let intent: JSONValue
        let cases: [ReferenceCase]

        enum CodingKeys: String, CodingKey {
            case schemaVersion = "schema_version"
            case intent, cases
        }
    }

    private struct ReferenceCase: Decodable {
        let targetCollection: String
        let targetID: String
        let references: [ExpectedReference]

        enum CodingKeys: String, CodingKey {
            case targetCollection = "target_collection"
            case targetID = "target_id"
            case references
        }
    }

    private struct ExpectedReference: Decodable, Equatable {
        let sourceCollection: String
        let sourceObjectType: String
        let sourceID: String
        let field: String

        enum CodingKeys: String, CodingKey {
            case sourceCollection = "source_collection"
            case sourceObjectType = "source_object_type"
            case sourceID = "source_id"
            case field
        }
    }

    private struct RuleDocument: Decodable {
        let schemaVersion: Int
        let cases: [RuleCase]
        enum CodingKeys: String, CodingKey {
            case schemaVersion = "schema_version"
            case cases
        }
    }

    private struct RuleCase: Decodable {
        let name: String
        let rule: JSONValue
        let tokens: [String]
        let dnsContinues: Bool
        enum CodingKeys: String, CodingKey {
            case name, rule, tokens
            case dnsContinues = "dns_continues"
        }
    }

    private struct ValidationDocument: Decodable {
        let schemaVersion: Int
        let validation: ValidationResult
        let forbiddenMessageValues: [String]
        enum CodingKeys: String, CodingKey {
            case schemaVersion = "schema_version"
            case validation
            case forbiddenMessageValues = "forbidden_message_values"
        }
    }

    private var repositoryRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }

    private func decode<T: Decodable>(_ type: T.Type, _ path: String) throws -> T {
        let data = try Data(contentsOf: repositoryRoot.appendingPathComponent(path))
        return try JSONDecoder().decode(type, from: data)
    }

    func testSharedReferenceGuardContractMatchesSwift() throws {
        let document = try decode(ReferenceDocument.self, "ui/collection-reference-fixtures.json")
        XCTAssertEqual(document.schemaVersion, 1)
        let root = try XCTUnwrap(document.intent.objectValue)
        for fixture in document.cases {
            let actual = SteerUISpec.inboundReferences(
                root: root, targetCollection: fixture.targetCollection, targetID: fixture.targetID
            ).map {
                ExpectedReference(
                    sourceCollection: $0.sourceCollection,
                    sourceObjectType: $0.sourceObjectType,
                    sourceID: $0.sourceID,
                    field: $0.field
                )
            }
            XCTAssertEqual(actual, fixture.references, "reference drift for \(fixture.targetCollection)/\(fixture.targetID)")
        }
    }

    func testSharedRuleSummaryAndDNSBoundaryMatchSwift() throws {
        let document = try decode(RuleDocument.self, "ui/rule-summary-fixtures.json")
        XCTAssertEqual(document.schemaVersion, 1)
        for fixture in document.cases {
            let rule = try XCTUnwrap(fixture.rule.objectValue)
            XCTAssertEqual(SteerUISpec.ruleSummaryTokens(rule), fixture.tokens, fixture.name)
            XCTAssertEqual(SteerUISpec.ruleDNSContinues(rule), fixture.dnsContinues, fixture.name)
        }
        XCTAssertFalse(SteerUISpec.contract.platformCapabilities["macos"]?.sourceMAC ?? true)
        XCTAssertFalse(SteerUISpec.contract.platformCapabilities["macos"]?.sourceMACReason?.isEmpty ?? true)
    }

    func testSharedValidationIssuesPreserveLocationWithoutSecrets() throws {
        let document = try decode(ValidationDocument.self, "ui/validation-issue-fixtures.json")
        XCTAssertEqual(document.schemaVersion, 1)
        let issues = document.validation.errors + document.validation.warnings
        XCTAssertEqual(Set(issues.compactMap(\.objectType)), Set(["node", "route", "rule", "dns_profile", "local_proxy", "subscription"]))
        XCTAssertTrue(issues.allSatisfy { $0.objectID?.isEmpty == false && $0.option?.isEmpty == false })
        let rendered = issues.map { "\($0.code) \($0.message)" }.joined(separator: "\n")
        for secret in document.forbiddenMessageValues { XCTAssertFalse(rendered.contains(secret)) }
    }
}
