// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation
import XCTest
@testable import SteerApp

final class SystemComponentsStatusTests: XCTestCase {
    private struct Document: Decodable {
        let schemaVersion: Int
        let requiredComponents: [Component]
        let cases: [Fixture]

        enum CodingKeys: String, CodingKey {
            case schemaVersion = "schema_version"
            case requiredComponents = "required_components"
            case cases
        }
    }

    private struct Component: Decodable {
        let id: String
        let label: String
        let path: String
    }

    private struct Fixture: Decodable {
        let name: String
        let missing: [String]
        let outdated: [String]
    }

    private var repositoryRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }

    func testEveryRequiredComponentCanIndependentlyMakeInstallationIncomplete() throws {
        let data = try Data(contentsOf: repositoryRoot.appendingPathComponent("ui/macos-system-component-fixtures.json"))
        let document = try JSONDecoder().decode(Document.self, from: data)
        XCTAssertEqual(document.schemaVersion, 1)
        XCTAssertEqual(Set(document.cases.dropFirst().flatMap(\.missing)), Set(document.requiredComponents.map(\.id)))

        for fixture in document.cases {
            let facts = document.requiredComponents.map { component in
                let state: SystemComponentFact.State
                if fixture.missing.contains(component.id) {
                    state = .missing
                } else if fixture.outdated.contains(component.id) {
                    state = .outdated
                } else {
                    state = .ready
                }
                return SystemComponentFact(
                    id: component.id, label: component.label, path: component.path,
                    state: state, detail: state == .ready ? "ready" : state.rawValue
                )
            }
            let status = SystemComponentsStatus(
                facts: facts,
                embeddedInstallerAvailable: true,
                embeddedUninstallerAvailable: true,
                hasInstalledArtifacts: true
            )
            XCTAssertEqual(status.installed, fixture.name == "complete", fixture.name)
            XCTAssertEqual(status.updateAvailable, !fixture.outdated.isEmpty, fixture.name)
            XCTAssertEqual(Set(status.issues.map(\.id)), Set(fixture.missing + fixture.outdated), fixture.name)
        }
    }
}
