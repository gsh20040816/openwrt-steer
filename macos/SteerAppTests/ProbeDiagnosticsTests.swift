// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation
import XCTest
@testable import SteerApp

final class ProbeDiagnosticsTests: XCTestCase {
    private struct FixtureDocument: Decodable {
        let schemaVersion: Int
        let capability: [String: String]
        let objects: FixtureObjects
        let diagnostics: ProbeDiagnostics

        enum CodingKeys: String, CodingKey {
            case schemaVersion = "schema_version"
            case capability, objects, diagnostics
        }
    }

    private struct FixtureObjects: Decodable {
        let nodes: [FixtureObject]
        let routes: [FixtureObject]
        let subscriptions: [FixtureObject]
    }

    private struct FixtureObject: Decodable {
        let id: String
        let enabled: Bool
    }

    private func fixture() throws -> FixtureDocument {
        let repositoryRoot = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        let url = repositoryRoot.appendingPathComponent("ui/probe-diagnostics-fixtures.json")
        return try JSONDecoder().decode(FixtureDocument.self, from: Data(contentsOf: url))
    }

    func testSharedProbeFixtureKeepsFullSanitizedReportsAndAccurateBoundary() throws {
        let document = try fixture()
        XCTAssertEqual(document.schemaVersion, 1)
        XCTAssertTrue(document.capability["overview"]?.contains("does not prove a particular outbound") == true)
        XCTAssertEqual(document.objects.nodes.map(\.enabled), [true, false])
        XCTAssertEqual(document.objects.routes.map(\.enabled), [true, false])
        XCTAssertEqual(document.objects.subscriptions.map(\.enabled), [true, false])
        XCTAssertEqual(document.diagnostics.reports.map(\.scope), ["overview", "nodes", "routes"])
        XCTAssertEqual(document.diagnostics.dnsCapture?.mode, "dedicated_shim")
        XCTAssertEqual(document.diagnostics.dnsCapture?.activeGeneration, "generation-a")
        XCTAssertEqual(document.diagnostics.dnsCapture?.configured, true)
        XCTAssertTrue(document.diagnostics.dnsCapture?.detail.contains("port-53 capture artifacts") == true)

        let overview = document.diagnostics.reports[0]
        XCTAssertEqual(overview.results.first?.url, "https://probe.example/REDACTED")
        XCTAssertEqual(overview.results.first?.attempts, 1)
        XCTAssertEqual(overview.results.first?.connectMilliseconds, 7)
        XCTAssertEqual(overview.results.first?.tlsMilliseconds, 9)
        XCTAssertEqual(overview.results.first?.firstByteMilliseconds, 21)
        XCTAssertFalse(overview.isStale(relativeTo: RuntimeStatus(
            healthy: true, generationID: "generation-a", intentDigest: "active-a", error: ""
        )))

        let download = document.diagnostics.reports[1].results[0]
        XCTAssertEqual(download.downloadedBytes, 1_000_000)
        XCTAssertEqual(download.downloadMilliseconds, 500)
        XCTAssertEqual(document.diagnostics.reports[2].error, "probe timed out")
    }
}

private extension RuntimeStatus {
    init(healthy: Bool, generationID: String, intentDigest: String, error: String) {
        self.init()
        self.healthy = healthy
        self.generationID = generationID
        self.intentDigest = intentDigest
        self.error = error
    }
}
