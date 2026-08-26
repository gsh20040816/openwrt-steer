// SPDX-License-Identifier: GPL-3.0-or-later

import XCTest
@testable import SteerApp

private actor DraftOperationGate {
    private var operationStarted = false
    private var released = false
    private var operationContinuation: CheckedContinuation<Void, Never>?
    private var startedContinuation: CheckedContinuation<Void, Never>?

    func pause() async {
        operationStarted = true
        startedContinuation?.resume()
        startedContinuation = nil
        guard !released else { return }
        await withCheckedContinuation { continuation in
            operationContinuation = continuation
        }
    }

    func waitUntilStarted() async {
        if operationStarted { return }
        await withCheckedContinuation { continuation in
            startedContinuation = continuation
        }
    }

    func release() {
        released = true
        operationContinuation?.resume()
        operationContinuation = nil
    }
}

private actor DraftLifecycleBackend: BackendClient {
    private var snapshot: ConfigurationSnapshot
    private var installed: Bool
    private var runtimeStatus: RuntimeStatus
    private var loadCount = 0
    private var saveCount = 0
    private var applyCount = 0
    private var installCount = 0
    private var probeCount = 0
    private var failNextApply = false
    private var loadFailuresRemaining = 0
    private var nextLoadGate: DraftOperationGate?
    private var nextSaveGate: DraftOperationGate?
    private var nextApplyGate: DraftOperationGate?
    private var nextInstallGate: DraftOperationGate?

    init(document: String, revision: String = "revision-1", installed: Bool = true) {
        snapshot = ConfigurationSnapshot(document: document, revision: revision)
        self.installed = installed
        var status = RuntimeStatus()
        status.healthy = true
        status.generationID = "active-original"
        status.intentDigest = "active-original"
        runtimeStatus = status
    }

    func componentStatus() async -> SystemComponentsStatus {
        SystemComponentsStatus(
            installed: installed,
            embeddedInstallerAvailable: true,
            updateAvailable: installed
        )
    }

    func installSystemComponents() async throws {
        installCount += 1
        if let gate = nextInstallGate {
            nextInstallGate = nil
            await gate.pause()
        }
        installed = true
    }

    func validate(document: String) async throws -> ValidationResult {
        ValidationResult(ok: true, errors: [], warnings: [])
    }

    func loadConfiguration() async throws -> ConfigurationSnapshot {
        loadCount += 1
        if let gate = nextLoadGate {
            nextLoadGate = nil
            await gate.pause()
        }
        if loadFailuresRemaining > 0 {
            loadFailuresRemaining -= 1
            throw BackendClientError.processFailed("injected load failure")
        }
        return snapshot
    }

    func save(document: String, expectedRevision: String) async throws -> String {
        if let gate = nextSaveGate {
            nextSaveGate = nil
            await gate.pause()
        }
        guard expectedRevision == snapshot.revision else {
            throw BackendClientError.revisionConflict(currentRevision: snapshot.revision)
        }
        saveCount += 1
        let revision = "revision-saved-\(saveCount)"
        snapshot = ConfigurationSnapshot(document: document, revision: revision)
        return revision
    }

    func apply(document: String, expectedRevision: String) async throws -> ApplyOutcome {
        if let gate = nextApplyGate {
            nextApplyGate = nil
            await gate.pause()
        }
        guard expectedRevision == snapshot.revision else {
            throw BackendClientError.revisionConflict(currentRevision: snapshot.revision)
        }
        applyCount += 1
        let revision = "revision-applied-\(applyCount)"
        snapshot = ConfigurationSnapshot(document: document, revision: revision)
        if failNextApply {
            failNextApply = false
            return ApplyOutcome(
                status: runtimeStatus,
                saved: true,
                applied: false,
                revision: revision,
                error: "activation failed"
            )
        }
        runtimeStatus.generationID = "active-\(applyCount)"
        runtimeStatus.intentDigest = "active-\(applyCount)"
        return ApplyOutcome(
            status: runtimeStatus,
            saved: true,
            applied: true,
            revision: revision,
            error: ""
        )
    }

    func status() async throws -> RuntimeStatus { runtimeStatus }
    func logs() async throws -> String { "" }
    func versions() async throws -> RuntimeVersions { RuntimeVersions() }
    func parseNodes(document: String) async throws -> NodeImportResult { NodeImportResult(nodes: [], skipped: 0) }
    func probe(kind: String, nodeID: String?, routeID: String?, download: Bool) async throws -> ProbeReport {
        probeCount += 1
        return ProbeReport(
            ok: true,
            scope: nodeID == nil && routeID == nil ? "overview" : (nodeID == nil ? "routes" : "nodes"),
            objectID: nodeID ?? routeID,
            kind: kind,
            results: [ProbeResult(
                ok: true, status: 204, firstByteMilliseconds: 12,
                connectMilliseconds: 4, tlsMilliseconds: 6,
                downloadedBytes: nil, downloadMilliseconds: nil, error: nil
            )],
            error: nil,
            activeGeneration: nodeID == nil && routeID == nil ? runtimeStatus.generationID : nil,
            activeDigest: nodeID == nil && routeID == nil ? runtimeStatus.intentDigest : nil,
            testedAt: "2026-08-26T01:02:03Z"
        )
    }
    func subscriptionStatuses() async throws -> [SubscriptionRuntimeStatus] { [] }
    func updateSubscription(id: String) async throws {}
    func cleanSubscription(id: String, nodeID: String) async throws {}
    func geoCatalog(kind: String) async throws -> [String] { [] }

    func counts() -> (loads: Int, saves: Int, applies: Int, installs: Int, probes: Int) {
        (loadCount, saveCount, applyCount, installCount, probeCount)
    }

    func savedSnapshot() -> ConfigurationSnapshot { snapshot }
    func failUpcomingApply() { failNextApply = true }
    func failUpcomingLoad() { loadFailuresRemaining += 1 }
    func pauseUpcomingLoad(with gate: DraftOperationGate) { nextLoadGate = gate }
    func pauseUpcomingSave(with gate: DraftOperationGate) { nextSaveGate = gate }
    func pauseUpcomingApply(with gate: DraftOperationGate) { nextApplyGate = gate }
    func pauseUpcomingInstall(with gate: DraftOperationGate) { nextInstallGate = gate }
}

@MainActor
final class AppStateDraftLifecycleTests: XCTestCase {
    private let savedDocument = """
    {"main":{"schema_version":1,"enabled":false},"nodes":[],"routes":[],"dns_profiles":[],"rules":[],"subscriptions":[],"local_proxies":[]}
    """

    private let editedDocument = """
    {"main":{"schema_version":1,"enabled":false,"log_level":"debug"},"nodes":[],"routes":[],"dns_profiles":[],"rules":[],"subscriptions":[],"local_proxies":[]}
    """

    private let newerDocument = """
    {"main":{"schema_version":1,"enabled":false,"log_level":"error"},"nodes":[],"routes":[],"dns_profiles":[],"rules":[],"subscriptions":[],"local_proxies":[]}
    """

    func testInitialLoadRunsOnceAndWindowReopenPreservesDirtyDraft() async throws {
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)

        model.loadInitialState()
        try await waitUntil { !model.isBusy && model.hasInitializedDraft }
        model.rawJSON = editedDocument
        model.markDirty()

        model.loadInitialState()
        try await Task.sleep(nanoseconds: 30_000_000)

        let counts = await backend.counts()
        XCTAssertEqual(counts.loads, 1)
        XCTAssertEqual(model.rawJSON, editedDocument)
        XCTAssertTrue(model.isDirty)
    }

    func testInitialLoadPreservesEditsMadeWhileLoadIsInFlight() async throws {
        let gate = DraftOperationGate()
        let backend = DraftLifecycleBackend(document: savedDocument)
        await backend.pauseUpcomingLoad(with: gate)
        let model = AppModel(backend: backend)

        model.loadInitialState()
        await gate.waitUntilStarted()
        model.rawJSON = editedDocument
        model.markDirty()
        await gate.release()
        try await waitUntil { !model.isBusy && model.hasInitializedDraft }

        XCTAssertEqual(model.rawJSON, editedDocument)
        XCTAssertEqual(model.savedRevision, "revision-1")
        XCTAssertTrue(model.isDirty)
    }

    func testInitialLoadFailureCanRetryWithoutOverwritingLaterEdits() async throws {
        let backend = DraftLifecycleBackend(document: savedDocument)
        await backend.failUpcomingLoad()
        let model = AppModel(backend: backend)

        model.loadInitialState()
        try await waitUntil { !model.isBusy }
        XCTAssertFalse(model.hasInitializedDraft)
        model.rawJSON = editedDocument
        model.markDirty()

        model.loadInitialState()
        try await waitUntil { !model.isBusy && model.hasInitializedDraft }

        XCTAssertEqual(model.rawJSON, editedDocument)
        XCTAssertEqual(model.savedRevision, "revision-1")
        XCTAssertTrue(model.isDirty)
    }

    func testDirtyReloadCancelChangesNothingAndDiscardReloadsSaved() async throws {
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)
        model.loadDraft()
        try await waitUntil { !model.isBusy && model.savedRevision == "revision-1" }
        model.rawJSON = editedDocument
        model.markDirty()
        let activeBefore = model.runtime.generationID

        model.loadDraft()
        XCTAssertEqual(model.pendingDraftAction, .reload)
        XCTAssertFalse(model.canSaveDraft)
        XCTAssertFalse(model.canSaveAndApplyDraft)
        model.saveDraft()
        model.saveAndApplyDraft()
        model.setDraftValue(in: "main", key: "log_level", value: .string("warn"))
        let importedWhileGuardPending = await model.importNodes("vless://ignored")
        XCTAssertFalse(importedWhileGuardPending)
        model.resolveDraftGuard(.cancel)

        XCTAssertEqual(model.rawJSON, editedDocument)
        XCTAssertEqual(model.savedRevision, "revision-1")
        XCTAssertEqual(model.runtime.generationID, activeBefore)
        XCTAssertTrue(model.isDirty)
        XCTAssertNil(model.pendingDraftAction)
        var counts = await backend.counts()
        XCTAssertEqual(counts.loads, 1)
        XCTAssertEqual(counts.saves, 0)
        XCTAssertEqual(counts.applies, 0)

        model.loadDraft()
        model.resolveDraftGuard(.discard)
        try await waitUntil { !model.isBusy && !model.isDirty }
        counts = await backend.counts()
        XCTAssertEqual(counts.loads, 2)
        XCTAssertEqual(model.rawJSON, savedDocument)
    }

    func testDirtyReloadSaveWritesBeforeReplacingDraft() async throws {
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)
        model.loadDraft()
        try await waitUntil { !model.isBusy && model.savedRevision == "revision-1" }
        model.rawJSON = editedDocument
        model.markDirty()

        model.loadDraft()
        model.resolveDraftGuard(.save)
        try await waitUntil { !model.isBusy && !model.isDirty }

        let counts = await backend.counts()
        let saved = await backend.savedSnapshot()
        XCTAssertEqual(counts.saves, 1)
        XCTAssertEqual(counts.applies, 0)
        XCTAssertEqual(saved.document, editedDocument)
        XCTAssertEqual(model.rawJSON, editedDocument)
        XCTAssertTrue(model.canApplySaved)
    }

    func testReloadPreservesEditsMadeWhileReadIsInFlight() async throws {
        let gate = DraftOperationGate()
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)
        model.loadDraft()
        try await waitUntil { !model.isBusy && model.savedRevision == "revision-1" }
        await backend.pauseUpcomingLoad(with: gate)

        model.loadDraft()
        await gate.waitUntilStarted()
        model.rawJSON = editedDocument
        model.markDirty()
        await gate.release()
        try await waitUntil { !model.isBusy }

        XCTAssertEqual(model.rawJSON, editedDocument)
        XCTAssertTrue(model.isDirty)
        XCTAssertTrue(model.message.contains("已保留"))
    }

    func testSaveDoesNotMarkNewerInFlightEditsClean() async throws {
        let gate = DraftOperationGate()
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)
        model.loadDraft()
        try await waitUntil { !model.isBusy && model.savedRevision == "revision-1" }
        model.rawJSON = editedDocument
        model.markDirty()
        await backend.pauseUpcomingSave(with: gate)

        model.saveDraft()
        await gate.waitUntilStarted()
        model.rawJSON = newerDocument
        model.markDirty()
        await gate.release()
        try await waitUntil { !model.isBusy }

        let saved = await backend.savedSnapshot()
        XCTAssertEqual(saved.document, editedDocument)
        XCTAssertEqual(model.rawJSON, newerDocument)
        XCTAssertEqual(model.savedRevision, saved.revision)
        XCTAssertTrue(model.isDirty)
    }

    func testFirstInstallCanSaveAndPreserveAnEditedDraft() async throws {
        let backend = DraftLifecycleBackend(document: savedDocument, installed: false)
        let model = AppModel(backend: backend)
        model.loadInitialState()
        try await waitUntil { !model.isBusy && model.hasInitializedDraft }
        model.rawJSON = editedDocument
        model.markDirty()

        model.installSystemComponents()
        XCTAssertEqual(model.pendingDraftAction, .installSystemComponents)
        XCTAssertTrue(model.canSaveForPendingDraftAction)
        model.resolveDraftGuard(.cancel)
        var counts = await backend.counts()
        XCTAssertEqual(counts.installs, 0)
        XCTAssertEqual(model.rawJSON, editedDocument)
        XCTAssertTrue(model.isDirty)

        model.installSystemComponents()
        model.resolveDraftGuard(.save)
        try await waitUntil { !model.isBusy && model.systemComponentsInstalled && !model.isDirty }

        counts = await backend.counts()
        let saved = await backend.savedSnapshot()
        XCTAssertEqual(counts.installs, 1)
        XCTAssertEqual(counts.saves, 1)
        XCTAssertEqual(counts.applies, 0)
        XCTAssertEqual(saved.document, editedDocument)
        XCTAssertEqual(model.rawJSON, editedDocument)
        XCTAssertTrue(model.canApplySaved)
    }

    func testInstallPreservesNewEditsMadeWhileAuthorizationIsInFlight() async throws {
        let gate = DraftOperationGate()
        let backend = DraftLifecycleBackend(document: savedDocument, installed: false)
        let model = AppModel(backend: backend)
        model.loadInitialState()
        try await waitUntil { !model.isBusy && model.hasInitializedDraft }
        model.rawJSON = editedDocument
        model.markDirty()
        await backend.pauseUpcomingInstall(with: gate)

        model.installSystemComponents()
        model.resolveDraftGuard(.save)
        await gate.waitUntilStarted()
        model.rawJSON = newerDocument
        model.markDirty()
        await gate.release()
        try await waitUntil { !model.isBusy && model.systemComponentsInstalled }

        let saved = await backend.savedSnapshot()
        XCTAssertEqual(saved.document, editedDocument)
        XCTAssertEqual(model.rawJSON, newerDocument)
        XCTAssertTrue(model.isDirty)
        XCTAssertTrue(model.message.contains("已保留"))
    }

    func testDirtyEnableNeverDeploysTheRestOfTheDraft() async throws {
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)
        model.loadDraft()
        try await waitUntil { !model.isBusy && model.savedRevision == "revision-1" }
        model.rawJSON = editedDocument
        model.markDirty()

        model.setEnabledAndApply(true)

        let counts = await backend.counts()
        XCTAssertEqual(counts.saves, 0)
        XCTAssertEqual(counts.applies, 0)
        XCTAssertEqual(model.rawJSON, editedDocument)
        XCTAssertTrue(model.isDirty)
        XCTAssertTrue(model.message.contains("不会静默部署"))
    }

    func testSaveApplySavedAndSaveAndApplyRemainSeparateActions() async throws {
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)
        model.loadDraft()
        try await waitUntil { !model.isBusy && model.savedRevision == "revision-1" }
        model.rawJSON = editedDocument
        model.markDirty()

        model.saveDraft()
        try await waitUntil { !model.isBusy && !model.isDirty }
        var counts = await backend.counts()
        XCTAssertEqual(counts.saves, 1)
        XCTAssertEqual(counts.applies, 0)
        XCTAssertTrue(model.canApplySaved)

        await backend.failUpcomingApply()
        model.applySaved()
        try await waitUntil { !model.isBusy }
        counts = await backend.counts()
        XCTAssertEqual(counts.applies, 1)
        XCTAssertEqual(model.runtime.generationID, "active-original")
        XCTAssertTrue(model.message.contains("Apply 失败"))

        model.saveAndApplyDraft()
        try await waitUntil { !model.isBusy }
        counts = await backend.counts()
        XCTAssertEqual(counts.applies, 2)
        XCTAssertEqual(model.runtime.generationID, "active-2")
    }

    func testSaveAndApplyDoesNotMarkNewerInFlightEditsClean() async throws {
        let gate = DraftOperationGate()
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)
        model.loadDraft()
        try await waitUntil { !model.isBusy && model.savedRevision == "revision-1" }
        model.rawJSON = editedDocument
        model.markDirty()
        await backend.pauseUpcomingApply(with: gate)

        model.saveAndApplyDraft()
        await gate.waitUntilStarted()
        model.rawJSON = newerDocument
        model.markDirty()
        await gate.release()
        try await waitUntil { !model.isBusy }

        let saved = await backend.savedSnapshot()
        XCTAssertEqual(saved.document, editedDocument)
        XCTAssertEqual(model.rawJSON, newerDocument)
        XCTAssertTrue(model.isDirty)
        XCTAssertEqual(model.runtime.generationID, "active-1")
    }

    func testApplySavedPreservesAnIndependentDirtyDraft() async throws {
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)
        model.loadDraft()
        try await waitUntil { !model.isBusy && model.savedRevision == "revision-1" }
        model.rawJSON = editedDocument
        model.markDirty()

        model.applySaved()
        try await waitUntil { !model.isBusy }

        let counts = await backend.counts()
        XCTAssertEqual(counts.applies, 1)
        XCTAssertEqual(model.rawJSON, editedDocument)
        XCTAssertEqual(model.savedRevision, "revision-1")
        XCTAssertTrue(model.isDirty)
    }

    func testOverviewProbeIdentityStaysCurrentAfterSaveAndExpiresAfterActiveChangesOrStops() async throws {
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)
        model.loadInitialState()
        try await waitUntil { !model.isBusy && model.hasInitializedDraft }

        model.runProbe(kind: "proxy")
        try await waitUntil {
            !model.overviewProbeInProgress("proxy") && model.overviewProbeDetail("proxy") != nil
        }
        XCTAssertEqual(model.overviewProbeSummary("proxy"), "12 ms")
        XCTAssertTrue(model.overviewProbeDetail("proxy")?.contains("Active generation active-original") == true)
        XCTAssertTrue(model.overviewProbeDetail("proxy")?.contains("digest active-original") == true)
        XCTAssertTrue(model.overviewProbeDetail("proxy")?.contains("tested_at 2026-08-26T01:02:03Z") == true)
        XCTAssertFalse(model.overviewProbeIsStale("proxy"))

        model.rawJSON = editedDocument
        model.markDirty()
        model.saveDraft()
        try await waitUntil { !model.isBusy && !model.isDirty }
        XCTAssertEqual(model.runtime.generationID, "active-original")
        XCTAssertFalse(model.overviewProbeIsStale("proxy"), "Save-only must not change Active probe identity")

        model.runtime.healthy = false
        XCTAssertFalse(model.hasActiveGeneration)
        XCTAssertTrue(model.overviewProbeIsStale("proxy"), "an unhealthy data plane must expire a green result")
        model.runtime.healthy = true
        XCTAssertTrue(model.hasActiveGeneration)
        XCTAssertFalse(model.overviewProbeIsStale("proxy"))

        model.applySaved()
        try await waitUntil { !model.isBusy }
        XCTAssertEqual(model.runtime.generationID, "active-1")
        XCTAssertTrue(model.overviewProbeIsStale("proxy"))
        XCTAssertTrue(model.overviewProbeSummary("proxy").contains("已过期"))

        model.runtime = RuntimeStatus()
        XCTAssertFalse(model.hasActiveGeneration)
        XCTAssertTrue(model.overviewProbeIsStale("proxy"))
        XCTAssertTrue(model.overviewProbeDetail("proxy")?.contains("已过期") == true)
    }

    func testDisabledAndDirtyNodeRouteProbesNeverReachBackendAndResumeAfterSave() async throws {
        let document = """
        {"main":{"schema_version":1,"enabled":true},"nodes":[{"id":"node-disabled","enabled":false,"type":"socks","server":"node.example","server_port":1080}],"routes":[{"id":"route-disabled","enabled":false,"kind":"single","node":"node-disabled"}],"dns_profiles":[],"rules":[],"subscriptions":[],"local_proxies":[]}
        """
        let backend = DraftLifecycleBackend(document: document)
        let model = AppModel(backend: backend)
        model.loadInitialState()
        try await waitUntil { !model.isBusy && model.hasInitializedDraft }

        model.runProbe(kind: "speedtest", nodeID: "node-disabled")
        model.runProbe(kind: "speedtest", routeID: "route-disabled")
        var counts = await backend.counts()
        XCTAssertEqual(counts.probes, 0)

        model.setDraftItemEnabled(in: "nodes", at: 0, enabled: true)
        model.runProbe(kind: "speedtest", nodeID: "node-disabled")
        counts = await backend.counts()
        XCTAssertEqual(counts.probes, 0, "dirty Draft must not probe the committed disabled Node")
        model.saveDraft()
        try await waitUntil { !model.isBusy && !model.isDirty }
        model.runProbe(kind: "speedtest", nodeID: "node-disabled")
        try await waitUntil { !model.probeInProgress(scope: "nodes", objectID: "node-disabled", download: false) }
        counts = await backend.counts()
        XCTAssertEqual(counts.probes, 1)

        model.setDraftItemEnabled(in: "routes", at: 0, enabled: true)
        model.saveDraft()
        try await waitUntil { !model.isBusy && !model.isDirty }
        model.runProbe(kind: "speedtest", routeID: "route-disabled")
        try await waitUntil { !model.probeInProgress(scope: "routes", objectID: "route-disabled", download: false) }
        counts = await backend.counts()
        XCTAssertEqual(counts.probes, 2)
    }

    func testTerminationGuardCancelAndSaveReplyWithoutApplying() async throws {
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)
        model.loadDraft()
        try await waitUntil { !model.isBusy && model.savedRevision == "revision-1" }
        model.rawJSON = editedDocument
        model.markDirty()
        var terminationReply: Bool?

        XCTAssertTrue(model.beginTerminationGuard { terminationReply = $0 })
        model.resolveDraftGuard(.cancel)
        XCTAssertEqual(terminationReply, false)
        XCTAssertEqual(model.rawJSON, editedDocument)
        XCTAssertTrue(model.isDirty)

        terminationReply = nil
        XCTAssertTrue(model.beginTerminationGuard { terminationReply = $0 })
        model.resolveDraftGuard(.save)
        try await waitUntil { terminationReply != nil }

        let counts = await backend.counts()
        XCTAssertEqual(terminationReply, true)
        XCTAssertEqual(counts.saves, 1)
        XCTAssertEqual(counts.applies, 0)
        XCTAssertFalse(model.isDirty)
    }

    func testTerminationSaveCancelsExitWhenNewEditsArriveInFlight() async throws {
        let gate = DraftOperationGate()
        let backend = DraftLifecycleBackend(document: savedDocument)
        let model = AppModel(backend: backend)
        model.loadDraft()
        try await waitUntil { !model.isBusy && model.savedRevision == "revision-1" }
        model.rawJSON = editedDocument
        model.markDirty()
        await backend.pauseUpcomingSave(with: gate)
        var terminationReply: Bool?

        XCTAssertTrue(model.beginTerminationGuard { terminationReply = $0 })
        model.resolveDraftGuard(.save)
        await gate.waitUntilStarted()
        model.rawJSON = newerDocument
        model.markDirty()
        await gate.release()
        try await waitUntil { terminationReply != nil }

        XCTAssertEqual(terminationReply, false)
        XCTAssertEqual(model.rawJSON, newerDocument)
        XCTAssertTrue(model.isDirty)
    }

    private func waitUntil(
        timeoutNanoseconds: UInt64 = 2_000_000_000,
        condition: @escaping @MainActor () -> Bool
    ) async throws {
        let deadline = DispatchTime.now().uptimeNanoseconds + timeoutNanoseconds
        while !condition() {
            if DispatchTime.now().uptimeNanoseconds >= deadline {
                XCTFail("timed out waiting for AppModel state")
                return
            }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
    }
}
