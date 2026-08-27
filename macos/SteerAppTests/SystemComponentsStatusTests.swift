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
        let requiredForInstallation: Bool?

        enum CodingKeys: String, CodingKey {
            case id, label, path
            case requiredForInstallation = "required_for_installation"
        }
    }

    private struct Fixture: Decodable {
        let name: String
        let missing: [String]
        let outdated: [String]
        let inactive: [String]?
        let installed: Bool?
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
        XCTAssertEqual(document.schemaVersion, 2)
        let requiredIDs = Set(document.requiredComponents.filter {
            $0.requiredForInstallation != false
        }.map(\.id))
        XCTAssertEqual(Set(document.cases.dropFirst().flatMap(\.missing)), requiredIDs)

        for fixture in document.cases {
            let facts = document.requiredComponents.map { component in
                let state: SystemComponentFact.State
                if fixture.missing.contains(component.id) {
                    state = .missing
                } else if fixture.outdated.contains(component.id) {
                    state = .outdated
                } else if fixture.inactive?.contains(component.id) == true {
                    state = .inactive
                } else {
                    state = .ready
                }
                return SystemComponentFact(
                    id: component.id, label: component.label, path: component.path,
                    state: state, detail: state == .ready ? "ready" : state.rawValue,
                    requiredForInstallation: component.requiredForInstallation != false
                )
            }
            let status = SystemComponentsStatus(
                facts: facts,
                embeddedInstallerAvailable: true,
                embeddedUninstallerAvailable: true,
                hasInstalledArtifacts: true
            )
            let expectedInstalled = fixture.installed ?? fixture.missing.isEmpty && fixture.outdated.isEmpty
            XCTAssertEqual(status.installed, expectedInstalled, fixture.name)
            XCTAssertEqual(status.updateAvailable, !fixture.outdated.isEmpty, fixture.name)
            XCTAssertEqual(
                Set(status.issues.map(\.id)),
                Set((fixture.missing + fixture.outdated).filter(requiredIDs.contains)),
                fixture.name
            )
        }
    }
}
