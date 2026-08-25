// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation
import SwiftUI

enum AppPage: String, CaseIterable, Identifiable {
    case overview = "Overview"
    case general = "General"
    case configuration = "Configuration"
    case nodes = "Nodes"
    case routes = "Routes"
    case dns = "DNS"
    case rules = "Rules"
    case subscriptions = "Subscriptions"
    case proxies = "Local Proxies"
    case diagnostics = "Diagnostics"
    case settings = "Settings"

    var id: String { rawValue }

    var systemImage: String {
        switch self {
        case .overview: return "circle.grid.2x2"
        case .general: return "switch.2"
        case .configuration: return "doc.text"
        case .nodes: return "point.3.connected.trianglepath.dotted"
        case .routes: return "arrow.triangle.branch"
        case .dns: return "network"
        case .rules: return "list.number"
        case .subscriptions: return "arrow.down.circle"
        case .proxies: return "rectangle.connected.to.line.below"
        case .diagnostics: return "stethoscope"
        case .settings: return "gearshape"
        }
    }
}

extension JSONValue {
    var objectValue: [String: JSONValue]? {
        guard case let .object(value) = self else { return nil }
        return value
    }

    var stringValue: String? {
        guard case let .string(value) = self else { return nil }
        return value
    }

    var boolValue: Bool? {
        guard case let .bool(value) = self else { return nil }
        return value
    }

    var numberValue: Double? {
        guard case let .number(value) = self else { return nil }
        return value
    }

    var arrayValue: [JSONValue]? {
        guard case let .array(value) = self else { return nil }
        return value
    }
}

struct RuntimeStatus: Decodable, Sendable {
    var healthy = false
    var generationID = ""
    var intentDigest = ""
    var error = ""

    init() {}

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        healthy = try container.decodeIfPresent(Bool.self, forKey: .healthy) ?? false
        generationID = try container.decodeIfPresent(String.self, forKey: .generationID) ?? ""
        intentDigest = try container.decodeIfPresent(String.self, forKey: .intentDigest) ?? ""
        error = try container.decodeIfPresent(String.self, forKey: .error) ?? ""
    }

    enum CodingKeys: String, CodingKey {
        case healthy
        case generationID = "generation_id"
        case intentDigest = "intent_digest"
        case error
    }
}

struct ValidationIssue: Codable, Identifiable {
    var id: String { "\(code):\(objectID ?? ""):\(option ?? "")" }
    let code: String
    let objectType: String?
    let objectID: String?
    let option: String?
    let message: String

    enum CodingKeys: String, CodingKey {
        case code
        case objectType = "object_type"
        case objectID = "object_id"
        case option
        case message
    }
}

struct ValidationResult: Codable {
    let ok: Bool
    let errors: [ValidationIssue]
    let warnings: [ValidationIssue]
}

struct RuntimeVersions {
    var helper = "—"
    var singBox = "—"
}

struct ApplyOutcome: Sendable {
    let status: RuntimeStatus
    let saved: Bool
    let applied: Bool
    let revision: String
    let error: String
}

struct ProbeResult: Decodable, Sendable {
    let ok: Bool
    let status: Int?
    let firstByteMilliseconds: Int?
    let connectMilliseconds: Int?
    let tlsMilliseconds: Int?
    let downloadedBytes: Int?
    let downloadMilliseconds: Int?
    let error: String?

    enum CodingKeys: String, CodingKey {
        case ok, status, error
        case firstByteMilliseconds = "first_byte_milliseconds"
        case connectMilliseconds = "connect_milliseconds"
        case tlsMilliseconds = "tls_milliseconds"
        case downloadedBytes = "downloaded_bytes"
        case downloadMilliseconds = "download_milliseconds"
    }
}

struct ProbeReport: Decodable, Sendable {
    let ok: Bool
    let scope: String
    let objectID: String?
    let kind: String
    let results: [ProbeResult]
    let error: String?

    enum CodingKeys: String, CodingKey {
        case ok, scope, kind, results, error
        case objectID = "object_id"
    }

    var summary: String {
        guard let result = results.first(where: { $0.ok }) else { return error ?? results.first?.error ?? "失败" }
        if let bytes = result.downloadedBytes, let milliseconds = result.downloadMilliseconds, milliseconds > 0 {
            return String(format: "%.1f Mbps", Double(bytes) * 8 / Double(milliseconds) / 1000)
        }
        if let milliseconds = result.firstByteMilliseconds ?? result.tlsMilliseconds ?? result.connectMilliseconds {
            return "\(milliseconds) ms"
        }
        return result.status.map { "HTTP \($0)" } ?? "成功"
    }
}

struct SubscriptionRuntimeStatus: Decodable, Identifiable, Sendable {
    let id: String
    let name: String?
    let url: String
    let enabled: Bool
    let updateInterval: String?
    let fetchedAt: String?
    let nodeCount: Int
    let skipped: Int
    let staleNodeIDs: [String]
    let error: String?

    enum CodingKeys: String, CodingKey {
        case id, name, url, enabled, skipped, error
        case updateInterval = "update_interval"
        case fetchedAt = "fetched_at"
        case nodeCount = "node_count"
        case staleNodeIDs = "stale_node_ids"
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decodeIfPresent(String.self, forKey: .name)
        url = try container.decode(String.self, forKey: .url)
        enabled = try container.decode(Bool.self, forKey: .enabled)
        updateInterval = try container.decodeIfPresent(String.self, forKey: .updateInterval)
        fetchedAt = try container.decodeIfPresent(String.self, forKey: .fetchedAt)
        nodeCount = try container.decodeIfPresent(Int.self, forKey: .nodeCount) ?? 0
        skipped = try container.decodeIfPresent(Int.self, forKey: .skipped) ?? 0
        staleNodeIDs = try container.decodeIfPresent([String].self, forKey: .staleNodeIDs) ?? []
        error = try container.decodeIfPresent(String.self, forKey: .error)
    }
}

private struct SubscriptionStatusResponse: Decodable {
    let ok: Bool
    let subscriptions: [SubscriptionRuntimeStatus]
}

private struct GeoCatalogResponse: Decodable {
    let kind: String
    let names: [String]
}

struct SystemComponentsStatus: Sendable {
    let installed: Bool
    let embeddedInstallerAvailable: Bool
    let updateAvailable: Bool
}

private struct ControlResponse: Decodable {
    let schemaVersion: Int
    let ok: Bool
    let status: RuntimeStatus?
    let saved: Bool?
    let applied: Bool?
    let revision: String?
    let payload: JSONValue?
    let error: String?

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case ok, status, saved, applied, revision, payload, error
    }
}

indirect enum JSONValue: Codable, Sendable {
    case object([String: JSONValue])
    case array([JSONValue])
    case string(String)
    case number(Double)
    case bool(Bool)
    case null

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if container.decodeNil() { self = .null; return }
        if let value = try? container.decode([String: JSONValue].self) { self = .object(value); return }
        if let value = try? container.decode([JSONValue].self) { self = .array(value); return }
        if let value = try? container.decode(String.self) { self = .string(value); return }
        if let value = try? container.decode(Bool.self) { self = .bool(value); return }
        self = .number(try container.decode(Double.self))
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case let .object(value): try container.encode(value)
        case let .array(value): try container.encode(value)
        case let .string(value): try container.encode(value)
        case let .number(value): try container.encode(value)
        case let .bool(value): try container.encode(value)
        case .null: try container.encodeNil()
        }
    }
}

struct NodeImportResult: Decodable, Sendable {
    let nodes: [JSONValue]
    let skipped: Int
}

enum BackendClientError: LocalizedError {
    case helperUnavailable
    case invalidResponse
    case validationFailed
    case processFailed(String)

    var errorDescription: String? {
        switch self {
        case .helperUnavailable:
            return "未安装 Steer 系统组件；请在“系统”页完成首次安装。"
        case .invalidResponse:
            return "steer-macos 返回了无法识别的响应。"
        case .validationFailed:
            return "配置校验失败，未保存也未 Apply。"
        case let .processFailed(message):
            return message
        }
    }
}

protocol BackendClient: Sendable {
    func componentStatus() async -> SystemComponentsStatus
    func installSystemComponents() async throws
    func validate(document: String) async throws -> ValidationResult
    func loadConfiguration() async throws -> String
    func save(document: String) async throws
    func apply(document: String) async throws -> ApplyOutcome
    func status() async throws -> RuntimeStatus
    func logs() async throws -> String
    func versions() async throws -> RuntimeVersions
    func parseNodes(document: String) async throws -> NodeImportResult
    func probe(kind: String, nodeID: String?, routeID: String?, download: Bool) async throws -> ProbeReport
    func subscriptionStatuses() async throws -> [SubscriptionRuntimeStatus]
    func updateSubscription(id: String) async throws
    func cleanSubscription(id: String, nodeID: String) async throws
    func geoCatalog(kind: String) async throws -> [String]
}

struct HelperBackendClient: BackendClient {
    private static let installedHelperPath = "/usr/local/libexec/steer/steer-macos"
    private static let configurationPath = "/Library/Application Support/Steer/config/config.json"
    private static let controlPlistPath = "/Library/LaunchDaemons/com.steer.steer.control.plist"
    private static let subscriptionPlistPath = "/Library/LaunchDaemons/com.steer.steer.subscription.plist"
    private static let installedSingBoxPath = "/usr/local/libexec/steer/sing-box"

    private let validationHelperURL: URL

    init(helperURL: URL? = nil) {
        validationHelperURL = helperURL
            ?? Self.embeddedInstallerResource("steer-macos")
            ?? URL(fileURLWithPath: Self.installedHelperPath)
    }

    func componentStatus() async -> SystemComponentsStatus {
        let fileManager = FileManager.default
        let installed = fileManager.isExecutableFile(atPath: Self.installedHelperPath)
            && fileManager.isExecutableFile(atPath: Self.installedSingBoxPath)
            && fileManager.fileExists(atPath: Self.controlPlistPath)
            && fileManager.fileExists(atPath: Self.subscriptionPlistPath)
        let embeddedHelper = Self.embeddedInstallerResource("steer-macos")
        let installerAvailable = embeddedInstallerURL.map {
            fileManager.isExecutableFile(atPath: $0.path)
        } ?? false
        var updateAvailable = false
        if installed, let embeddedHelper,
           fileManager.isExecutableFile(atPath: embeddedHelper.path) {
            let installedHelper = URL(fileURLWithPath: Self.installedHelperPath)
            if let installedVersion = try? await Self.execute(installedHelper, ["version"]),
               let embeddedVersion = try? await Self.execute(embeddedHelper, ["version"]) {
                updateAvailable = installedVersion.stdout != embeddedVersion.stdout
            }
        }
        return SystemComponentsStatus(
            installed: installed,
            embeddedInstallerAvailable: installerAvailable,
            updateAvailable: updateAvailable
        )
    }

    func installSystemComponents() async throws {
        guard let installer = embeddedInstallerURL,
              FileManager.default.isExecutableFile(atPath: installer.path) else {
            throw BackendClientError.processFailed("当前 App 不包含正式的系统组件 payload；源码开发请运行 macos/scripts/install-launchdaemon.sh。")
        }
        _ = try await Self.executePrivileged(Self.command([installer.path]))
    }

    func validate(document: String) async throws -> ValidationResult {
        try requireExecutable(validationHelperURL)
        return try await withTemporaryDocument(document) { url in
            let result = try await Self.execute(validationHelperURL, ["validate", "--config", url.path])
            guard let validation = try? JSONDecoder().decode(ValidationResult.self, from: result.stdout) else {
                throw result.error
            }
            return validation
        }
    }

    func loadConfiguration() async throws -> String {
        let url = URL(fileURLWithPath: Self.configurationPath)
        let output: Data
        do {
            output = try await Task.detached { try Data(contentsOf: url) }.value
        } catch {
            throw BackendClientError.processFailed(
                "无法读取系统配置。请重新运行 macos/scripts/install-launchdaemon.sh 一次以更新只读权限。"
            )
        }
        guard let document = String(data: output, encoding: .utf8) else {
            throw BackendClientError.invalidResponse
        }
        return document
    }

    func save(document: String) async throws {
        let validation = try await validate(document: document)
        guard validation.ok else { throw BackendClientError.validationFailed }
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        try requireExecutable(helper)
        try await withTemporaryDocument(document) { url in
            let result = try await Self.execute(helper, ["control", "--operation", "save", "--input", url.path])
            let response = try Self.decodeControlResponse(result)
            guard response.ok, response.saved == true else {
                throw BackendClientError.processFailed(response.error ?? "Steer control service 拒绝保存配置。")
            }
        }
    }

    func apply(document: String) async throws -> ApplyOutcome {
        let validation = try await validate(document: document)
        guard validation.ok else { throw BackendClientError.validationFailed }
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        try requireExecutable(helper)
        return try await withTemporaryDocument(document) { url in
            let result = try await Self.execute(helper, ["control", "--operation", "apply", "--input", url.path])
            let response = try Self.decodeControlResponse(result)
            if response.saved != true {
                throw BackendClientError.processFailed(response.error ?? "配置未保存。")
            }
            let status: RuntimeStatus
            if let responseStatus = response.status {
                status = responseStatus
            } else {
                status = try await self.status()
            }
            return ApplyOutcome(
                status: status, saved: true, applied: response.applied == true,
                revision: response.revision ?? "", error: response.error ?? ""
            )
        }
    }

    func status() async throws -> RuntimeStatus {
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        try requireExecutable(helper)
        let result = try await Self.execute(helper, ["status"])
        guard let status = try? JSONDecoder().decode(RuntimeStatus.self, from: result.stdout) else {
            throw result.status == 0 ? BackendClientError.invalidResponse : result.error
        }
        return status
    }

    func logs() async throws -> String {
        let sources = [
            ("control.error.log", "/Library/Logs/Steer/control.error.log"),
            ("control.log", "/Library/Logs/Steer/control.log"),
            ("sing-box.error.log", "/Library/Logs/Steer/sing-box.error.log"),
            ("sing-box.log", "/Library/Logs/Steer/sing-box.log"),
            ("subscription.error.log", "/Library/Logs/Steer/subscription.error.log"),
            ("subscription.log", "/Library/Logs/Steer/subscription.log"),
        ]
        var sections: [String] = []
        for (name, path) in sources {
            let result = try await Self.execute(
                URL(fileURLWithPath: "/usr/bin/tail"),
                ["-n", "120", path]
            )
            guard result.status == 0 else { continue }
            let body = String(decoding: result.stdout, as: UTF8.self)
                .replacingOccurrences(
                    of: "\u{001B}\\[[0-9;]*m",
                    with: "",
                    options: .regularExpression
                )
                .trimmingCharacters(in: .whitespacesAndNewlines)
            if !body.isEmpty { sections.append("== \(name) ==\n\(body)") }
        }
        return sections.joined(separator: "\n\n")
    }

    func versions() async throws -> RuntimeVersions {
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        let singBox = URL(fileURLWithPath: "/usr/local/libexec/steer/sing-box")
        try requireExecutable(helper)
        try requireExecutable(singBox)
        let helperResult = try await Self.execute(helper, ["version"])
        let singBoxResult = try await Self.execute(singBox, ["version", "--name"])
        return RuntimeVersions(
            helper: String(decoding: helperResult.stdout, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines),
            singBox: String(decoding: singBoxResult.stdout, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
        )
    }

    func parseNodes(document: String) async throws -> NodeImportResult {
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        try requireExecutable(helper)
        return try await withTemporaryDocument(document) { url in
            let result = try await Self.execute(helper, ["parse-nodes", "--input", url.path])
            guard let parsed = try? JSONDecoder().decode(NodeImportResult.self, from: result.stdout) else {
                throw result.status == 0 ? BackendClientError.invalidResponse : result.error
            }
            return parsed
        }
    }

    func probe(kind: String, nodeID: String?, routeID: String?, download: Bool) async throws -> ProbeReport {
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        try requireExecutable(helper)
        var arguments = ["probe", "--kind", kind]
        if let nodeID { arguments += ["--node", nodeID] }
        if let routeID { arguments += ["--route", routeID] }
        if download { arguments.append("--download") }
        let result = try await Self.execute(helper, arguments)
        guard let report = try? JSONDecoder().decode(ProbeReport.self, from: result.stdout) else {
            throw result.status == 0 ? BackendClientError.invalidResponse : result.error
        }
        return report
    }

    func subscriptionStatuses() async throws -> [SubscriptionRuntimeStatus] {
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        try requireExecutable(helper)
        let result = try await Self.execute(helper, ["subscription", "status"])
        guard let response = try? JSONDecoder().decode(SubscriptionStatusResponse.self, from: result.stdout), response.ok else {
            throw result.status == 0 ? BackendClientError.invalidResponse : result.error
        }
        return response.subscriptions
    }

    func updateSubscription(id: String) async throws {
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        try requireExecutable(helper)
        let result = try await Self.execute(helper, ["control", "--operation", "subscription-update", "--id", id])
        let response = try Self.decodeControlResponse(result)
        guard response.ok else { throw BackendClientError.processFailed(response.error ?? "订阅更新失败。") }
    }

    func cleanSubscription(id: String, nodeID: String) async throws {
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        try requireExecutable(helper)
        let result = try await Self.execute(helper, ["control", "--operation", "subscription-clean", "--id", id, "--node", nodeID])
        let response = try Self.decodeControlResponse(result)
        guard response.ok else { throw BackendClientError.processFailed(response.error ?? "stale 节点清理失败。") }
    }

    func geoCatalog(kind: String) async throws -> [String] {
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        try requireExecutable(helper)
        let result = try await Self.execute(helper, ["geo-catalog", "--kind", kind])
        guard let response = try? JSONDecoder().decode(GeoCatalogResponse.self, from: result.stdout) else {
            throw result.status == 0 ? BackendClientError.invalidResponse : result.error
        }
        return response.names
    }

    private func requireExecutable(_ url: URL) throws {
        guard FileManager.default.isExecutableFile(atPath: url.path) else {
            throw BackendClientError.helperUnavailable
        }
    }

    private var embeddedInstallerURL: URL? {
        Self.embeddedInstallerResource("install-embedded-payload.sh")
    }

    private static func embeddedInstallerResource(_ name: String) -> URL? {
        Bundle.main.resourceURL?
            .appendingPathComponent("Installer", isDirectory: true)
            .appendingPathComponent(name, isDirectory: false)
    }

    private static func decodeControlResponse(_ result: ProcessResult) throws -> ControlResponse {
        guard let response = try? JSONDecoder().decode(ControlResponse.self, from: result.stdout) else {
            throw result.status == 0 ? BackendClientError.invalidResponse : result.error
        }
        guard response.schemaVersion == 1 else {
            throw BackendClientError.invalidResponse
        }
        return response
    }

    private func withTemporaryDocument<T>(
        _ document: String,
        operation: (URL) async throws -> T
    ) async throws -> T {
        let url = FileManager.default.temporaryDirectory
            .appendingPathComponent("steer-draft-\(UUID().uuidString).json")
        try Data(document.utf8).write(to: url, options: .atomic)
        defer { try? FileManager.default.removeItem(at: url) }
        return try await operation(url)
    }

    private struct ProcessResult: Sendable {
        let stdout: Data
        let stderr: String
        let status: Int32

        var error: BackendClientError {
            let message = stderr.trimmingCharacters(in: .whitespacesAndNewlines)
            return .processFailed(message.isEmpty ? "steer-macos 执行失败（退出码 \(status)）。" : message)
        }
    }

    private static func execute(_ executable: URL, _ arguments: [String]) async throws -> ProcessResult {
        try await Task.detached {
            let process = Process()
            let stdout = Pipe()
            let stderr = Pipe()
            process.executableURL = executable
            process.arguments = arguments
            process.standardOutput = stdout
            process.standardError = stderr
            try process.run()
            let output = stdout.fileHandleForReading.readDataToEndOfFile()
            let errorOutput = stderr.fileHandleForReading.readDataToEndOfFile()
            process.waitUntilExit()
            return ProcessResult(
                stdout: output,
                stderr: String(decoding: errorOutput, as: UTF8.self),
                status: process.terminationStatus
            )
        }.value
    }

    private static func executePrivileged(_ shellCommand: String) async throws -> Data {
        let result = try await execute(URL(fileURLWithPath: "/usr/bin/osascript"), [
            "-e", "on run argv",
            "-e", "do shell script (item 1 of argv) with administrator privileges",
            "-e", "end run",
            shellCommand,
        ])
        guard result.status == 0 else { throw result.error }
        return result.stdout
    }

    private static func command(_ arguments: [String]) -> String {
        arguments.map(shellQuote).joined(separator: " ")
    }

    private static func shellQuote(_ value: String) -> String {
        "'" + value.replacingOccurrences(of: "'", with: "'\\''") + "'"
    }
}

@MainActor
final class AppModel: ObservableObject {
    @Published var selectedPage: AppPage = .overview
    @Published var rawJSON = "{\n  \"main\": {}\n}"
    @Published var runtime = RuntimeStatus()
    @Published var validation: ValidationResult?
    @Published var isDirty = false
    @Published var isBusy = false
    @Published var message = ""
    @Published var diagnosticsLog = ""
    @Published var versions = RuntimeVersions()
    @Published var systemComponentsInstalled = false
    @Published var embeddedInstallerAvailable = false
    @Published var systemComponentsUpdateAvailable = false
    @Published var subscriptionRuntime: [SubscriptionRuntimeStatus] = []
    @Published var probeSummaries: [String: String] = [:]
    @Published var geositeNames: [String] = []
    @Published var geoipNames: [String] = []

    private let backend: BackendClient

    init(backend: BackendClient? = nil) {
        self.backend = backend ?? HelperBackendClient()
    }

    var draftEnabled: Bool {
        parseDraft()?.objectValue?["main"]?.objectValue?["enabled"]?.boolValue ?? false
    }

    var draftSyntaxError: String? {
        guard let data = rawJSON.data(using: .utf8) else { return "配置无法编码为 UTF-8" }
        do {
            _ = try JSONDecoder().decode(JSONValue.self, from: data)
            return nil
        } catch {
            return error.localizedDescription
        }
    }

    var draftSchemaVersion: Int {
        Int(parseDraft()?.objectValue?["main"]?.objectValue?["schema_version"]?.numberValue ?? 0)
    }

    var draftLogLevel: String {
        parseDraft()?.objectValue?["main"]?.objectValue?["log_level"]?.stringValue ?? "—"
    }

    var draftDNSCacheCapacity: Int {
        Int(parseDraft()?.objectValue?["main"]?.objectValue?["dns_cache_capacity"]?.numberValue ?? 0)
    }

    func itemCount(for key: String) -> Int {
        parseDraft()?.objectValue?[key]?.arrayValue?.count ?? 0
    }

    func draftValue(in section: String, key: String) -> JSONValue? {
        parseDraft()?.objectValue?[section]?.objectValue?[key]
    }

    func setDraftValue(in section: String, key: String, value: JSONValue?) {
        guard !isBusy, var root = parseDraft()?.objectValue else { return }
        var object = root[section]?.objectValue ?? [:]
        if let value {
            object[key] = value
        } else {
            object.removeValue(forKey: key)
        }
        root[section] = .object(object)
        writeDraft(.object(root), context: "\(section).\(key)")
    }

    func enabledItemCount(for key: String) -> Int {
        draftItems(for: key).filter(\.enabled).count
    }

    func markDirty() {
        isDirty = true
        message = "有未应用的 draft 修改"
    }

    func loadInitialState() {
        perform(message: "正在连接 Steer 后端…") {
            let components = await self.backend.componentStatus()
            self.systemComponentsInstalled = components.installed
            self.embeddedInstallerAvailable = components.embeddedInstallerAvailable
            self.systemComponentsUpdateAvailable = components.updateAvailable
            guard components.installed else {
                self.message = components.embeddedInstallerAvailable
                    ? "尚未安装系统组件；请在“系统”页完成一次管理员授权安装"
                    : "尚未安装系统组件；当前为源码开发构建"
                return
            }
            self.rawJSON = try await self.backend.loadConfiguration()
            self.runtime = try await self.backend.status()
            self.versions = try await self.backend.versions()
            self.subscriptionRuntime = try await self.backend.subscriptionStatuses()
            self.geositeNames = try await self.backend.geoCatalog(kind: "geosite")
            self.geoipNames = try await self.backend.geoCatalog(kind: "geoip")
            self.isDirty = false
            self.message = self.runtime.healthy ? "Steer 运行正常" : "已连接后端，Steer 当前未运行"
        }
    }

    func installSystemComponents() {
        perform(message: "正在安装 Steer 系统组件；macOS 将请求一次管理员授权…") {
            try await self.backend.installSystemComponents()
            let components = await self.backend.componentStatus()
            self.systemComponentsInstalled = components.installed
            self.embeddedInstallerAvailable = components.embeddedInstallerAvailable
            self.systemComponentsUpdateAvailable = components.updateAvailable
            guard components.installed else {
                throw BackendClientError.processFailed("安装器已结束，但系统组件验收未通过。")
            }
            self.rawJSON = try await self.backend.loadConfiguration()
            self.runtime = try await self.backend.status()
            self.versions = try await self.backend.versions()
            self.subscriptionRuntime = try await self.backend.subscriptionStatuses()
            self.geositeNames = try await self.backend.geoCatalog(kind: "geosite")
            self.geoipNames = try await self.backend.geoCatalog(kind: "geoip")
            self.isDirty = false
            self.message = "系统组件安装完成；后续保存和应用不再请求管理员密码"
        }
    }

    func refreshStatus() {
        perform(message: "正在读取运行状态…") {
            self.runtime = try await self.backend.status()
            self.message = self.runtime.healthy ? "Steer 运行正常" : "Steer 当前未运行"
        }
    }

    func refreshLogs() {
        perform(message: "正在读取最近日志…") {
            self.diagnosticsLog = try await self.backend.logs()
            self.message = self.diagnosticsLog.isEmpty ? "当前没有日志输出" : "已读取最近日志"
        }
    }

    func importNodes(_ document: String) async -> Bool {
        guard !isBusy else { return false }
        isBusy = true
        message = "正在解析节点分享链接…"
        defer { isBusy = false }
        do {
            let result = try await backend.parseNodes(document: document)
            mutateCollection("nodes") { values in
                for value in result.nodes {
                    guard var object = value.objectValue else { continue }
                    object["id"] = .string("node-\(UUID().uuidString.lowercased().prefix(8))")
                    object.removeValue(forKey: "source_subscription")
                    object.removeValue(forKey: "source_fingerprint")
                    values.append(.object(object))
                }
            }
            message = result.skipped == 0
                ? "已导入 \(result.nodes.count) 个节点到工作副本"
                : "已导入 \(result.nodes.count) 个节点，跳过 \(result.skipped) 个无效条目"
            return true
        } catch {
            message = "导入节点失败：\(error.localizedDescription)"
            return false
        }
    }

    func validate() {
        perform(message: "正在校验…") {
            self.validation = try await self.backend.validate(document: self.rawJSON)
            self.message = self.validation?.ok == true ? "校验通过" : "校验发现问题"
        }
    }

    func apply() {
        perform(message: "正在保存并应用配置…") {
            let outcome = try await self.backend.apply(document: self.rawJSON)
            self.runtime = outcome.status
            self.isDirty = !outcome.saved
            if outcome.saved && !outcome.applied {
                self.message = "配置已保存，但 Apply 失败：\(outcome.error.isEmpty ? "运行态未切换" : outcome.error)"
            } else {
                self.message = self.runtime.healthy ? "配置已应用，Steer 运行正常" : "配置已保存，Steer 已停用"
            }
        }
    }

    func setEnabledAndApply(_ enabled: Bool) {
        guard !isBusy, enabled != draftEnabled else { return }
        guard var root = parseDraft()?.objectValue else {
            message = "当前工作副本不是合法的 JSON，无法切换运行状态"
            return
        }
        let previousDocument = rawJSON
        let previousDirty = isDirty
        var main = root["main"]?.objectValue ?? [:]
        main["enabled"] = .bool(enabled)
        root["main"] = .object(main)
        guard let data = try? JSONEncoder.pretty.encode(JSONValue.object(root)) else {
            message = "写回启用状态失败"
            return
        }

        let updatedDocument = String(decoding: data, as: UTF8.self)
        rawJSON = updatedDocument
        isBusy = true
        message = enabled ? "正在启用并应用 Steer…" : "正在停用并清理 Steer…"
        Task {
            defer { isBusy = false }
            do {
                let outcome = try await backend.apply(document: updatedDocument)
                runtime = outcome.status
                if outcome.saved {
                    isDirty = false
                }
                if outcome.applied {
                    message = enabled ? "Steer 已启用并应用" : "Steer 已停用并清理运行资源"
                } else if outcome.saved {
                    message = "启用状态已保存，但 Apply 失败：\(outcome.error)"
                } else {
                    throw BackendClientError.processFailed(outcome.error)
                }
            } catch {
                rawJSON = previousDocument
                isDirty = previousDirty
                message = "切换 Steer 状态失败：\(error.localizedDescription)"
            }
        }
    }

    func saveDraft() {
        perform(message: "正在保存配置…") {
            try await self.backend.save(document: self.rawJSON)
            self.isDirty = false
            self.message = "配置已保存；运行态未改变"
        }
    }

    func loadDraft() {
        perform(message: "正在读取配置…") {
            self.rawJSON = try await self.backend.loadConfiguration()
            self.isDirty = false
            self.message = "已读取系统配置"
        }
    }

    func runProbe(kind: String, nodeID: String? = nil, routeID: String? = nil, download: Bool = false) {
        let key = nodeID.map { "nodes:\($0):\(download ? "download" : "connect")" }
            ?? routeID.map { "routes:\($0):\(download ? "download" : "connect")" }
            ?? "overview:\(kind)"
        perform(message: "正在运行探测…") {
            let report = try await self.backend.probe(kind: kind, nodeID: nodeID, routeID: routeID, download: download)
            self.probeSummaries[key] = report.summary
            self.message = report.ok ? "探测完成：\(report.summary)" : "探测失败：\(report.summary)"
        }
    }

    func runAllNodeProbes(download: Bool) {
        perform(message: download ? "正在批量下载测速…" : "正在批量连接测试…") {
            let nodes = self.draftItems(for: "nodes").filter(\.enabled)
            var succeeded = 0
            for node in nodes {
                let report = try await self.backend.probe(kind: "speedtest", nodeID: node.identifier, routeID: nil, download: download)
                self.probeSummaries["nodes:\(node.identifier):\(download ? "download" : "connect")"] = report.summary
                if report.ok { succeeded += 1 }
            }
            self.message = "批量测试完成：成功 \(succeeded)/\(nodes.count)"
        }
    }

    func updateSubscription(_ id: String) {
        perform(message: "正在更新订阅…") {
            try await self.backend.updateSubscription(id: id)
            self.rawJSON = try await self.backend.loadConfiguration()
            self.subscriptionRuntime = try await self.backend.subscriptionStatuses()
            self.isDirty = false
            self.message = "订阅已更新；运行态未自动 Apply"
        }
    }

    func cleanSubscriptionNode(subscriptionID: String, nodeID: String) {
        perform(message: "正在清理 stale 节点…") {
            try await self.backend.cleanSubscription(id: subscriptionID, nodeID: nodeID)
            self.rawJSON = try await self.backend.loadConfiguration()
            self.subscriptionRuntime = try await self.backend.subscriptionStatuses()
            self.isDirty = false
            self.message = "已清理 stale 节点 \(nodeID)"
        }
    }

    func subscriptionStatus(_ id: String) -> SubscriptionRuntimeStatus? {
        subscriptionRuntime.first { $0.id == id }
    }

    func draftItems(for key: String) -> [DraftItem] {
        guard let root = parseDraft()?.objectValue,
              case let .array(values)? = root[key] else { return [] }
        return values.enumerated().map { index, value in
            let object = value.objectValue ?? [:]
            let identifier = object["id"]?.stringValue ?? "item-\(index + 1)"
            let name = object["name"]?.stringValue ?? ""
            let enabled = object["enabled"]?.boolValue ?? true
            let kind = object["type"]?.stringValue
                ?? object["kind"]?.stringValue
                ?? object["protocol"]?.stringValue
                ?? (object["default"]?.boolValue == true ? "default" : "")
            let detail = draftItemDetail(key: key, object: object)
            return DraftItem(
                id: "\(key):\(identifier)", index: index, identifier: identifier,
                title: name.isEmpty ? draftItemFallbackTitle(key: key, kind: kind) : name, kind: kind,
                detail: detail, enabled: enabled,
                subscriptionOwned: object["source_subscription"]?.stringValue?.isEmpty == false
            )
        }
    }

    func draftItemObject(for key: String, at index: Int) -> [String: JSONValue]? {
        guard let root = parseDraft()?.objectValue,
              case let .array(values)? = root[key], values.indices.contains(index) else { return nil }
        return values[index].objectValue
    }

    func newDraftItemObject(for key: String) -> [String: JSONValue]? {
        defaultItem(for: key).objectValue
    }

    @discardableResult
    func replaceDraftItem(in key: String, at index: Int, object: [String: JSONValue]) -> Bool {
        var replaced = false
        mutateCollection(key) { values in
            guard values.indices.contains(index) else { return }
            if key == "rules", object["default"]?.boolValue == true {
                values.remove(at: index)
                values.append(.object(object))
            } else {
                values[index] = .object(object)
            }
            replaced = true
        }
        return replaced
    }

    func setDraftItemEnabled(in key: String, at index: Int, enabled: Bool) {
        mutateCollection(key) { values in
            guard values.indices.contains(index), var object = values[index].objectValue else { return }
            object["enabled"] = .bool(enabled)
            values[index] = .object(object)
        }
    }

    func moveDraftItem(in key: String, at index: Int, offset: Int) {
        mutateCollection(key) { values in
            let destination = index + offset
            guard values.indices.contains(index), values.indices.contains(destination) else { return }
            values.swapAt(index, destination)
        }
    }

    @discardableResult
    func appendDraftItem(to key: String, object: [String: JSONValue]) -> Bool {
        mutateCollection(key) { values in
            if key == "rules", object["default"]?.boolValue != true,
               let defaultIndex = values.firstIndex(where: {
                   $0.objectValue?["default"]?.boolValue == true
               }) {
                values.insert(.object(object), at: defaultIndex)
            } else {
                values.append(.object(object))
            }
        }
        return true
    }

    func removeDraftItem(from key: String, at index: Int) {
        if key == "subscriptions",
           var root = parseDraft()?.objectValue,
           case var .array(subscriptions)? = root[key], subscriptions.indices.contains(index),
           let identifier = subscriptions[index].objectValue?["id"]?.stringValue {
            subscriptions.remove(at: index)
            root[key] = .array(subscriptions)
            if case let .array(nodes)? = root["nodes"] {
                root["nodes"] = .array(nodes.filter {
                    $0.objectValue?["source_subscription"]?.stringValue != identifier
                })
            }
            writeDraft(.object(root), context: key)
            return
        }
        mutateCollection(key) { values in
            guard values.indices.contains(index) else { return }
            values.remove(at: index)
        }
    }

    func deletionBlockReason(for key: String, at index: Int) -> String? {
        guard let root = parseDraft()?.objectValue,
              case let .array(values)? = root[key], values.indices.contains(index),
              let object = values[index].objectValue,
              let identifier = object["id"]?.stringValue else { return "项目已不存在" }

        switch key {
        case "nodes":
            let references = root["routes"]?.arrayValue?.filter {
                $0.objectValue?["node"]?.stringValue == identifier
            }.count ?? 0
            if references > 0 { return "仍有 \(references) 条路由使用这个节点" }
        case "routes":
            let kind = object["kind"]?.stringValue ?? ""
            if kind == "direct" || kind == "block" { return "Direct 和 Block 是系统路由，不能删除" }
            let ruleReferences = root["rules"]?.arrayValue?.filter {
                $0.objectValue?["route"]?.stringValue == identifier
            }.count ?? 0
            let detourReferences = root["routes"]?.arrayValue?.filter {
                $0.objectValue?["detour"]?.stringValue == identifier
            }.count ?? 0
            if ruleReferences + detourReferences > 0 {
                return "仍有 \(ruleReferences + detourReferences) 个对象使用这条路由"
            }
        case "dns_profiles":
            let references = root["rules"]?.arrayValue?.filter {
                $0.objectValue?["dns_profile"]?.stringValue == identifier
            }.count ?? 0
            if references > 0 { return "仍有 \(references) 条规则使用这个 DNS Profile" }
        case "local_proxies":
            let references = root["rules"]?.arrayValue?.filter {
                $0.objectValue?["inbound"]?.arrayValue?.contains(where: {
                    $0.stringValue == identifier
                }) == true
            }.count ?? 0
            if references > 0 { return "仍有 \(references) 条规则使用这个本地入口" }
        case "rules":
            if object["default"]?.boolValue == true { return "Default 规则必须保留" }
        default:
            break
        }
        return nil
    }

    func moveDraftItem(in key: String, from source: IndexSet, to destination: Int) {
        mutateCollection(key) { values in
            values.move(fromOffsets: source, toOffset: destination)
        }
    }

    private func perform(message pendingMessage: String, operation: @escaping () async throws -> Void) {
        guard !isBusy else { return }
        isBusy = true
        message = pendingMessage
        Task {
            defer { isBusy = false }
            do {
                try await operation()
            } catch {
                message = error.localizedDescription
            }
        }
    }

    private func parseDraft() -> JSONValue? {
        guard let data = rawJSON.data(using: .utf8) else { return nil }
        return try? JSONDecoder().decode(JSONValue.self, from: data)
    }

    private func mutateCollection(_ key: String, _ mutate: (inout [JSONValue]) -> Void) {
        guard var root = parseDraft()?.objectValue else {
            message = "当前 draft 不是 JSON object，无法修改 \(key)"
            return
        }
        var values: [JSONValue]
        if case let .array(existing)? = root[key] { values = existing } else { values = [] }
        mutate(&values)
        root[key] = .array(values)
        writeDraft(.object(root), context: key)
    }

    private func writeDraft(_ value: JSONValue, context: String) {
        do {
            let data = try JSONEncoder.pretty.encode(value)
            rawJSON = String(decoding: data, as: UTF8.self)
            markDirty()
        } catch {
            message = "写回 \(context) 失败：\(error.localizedDescription)"
        }
    }

    private func defaultItem(for key: String) -> JSONValue {
        let prefix: String
        switch key {
        case "nodes": prefix = "node"
        case "routes": prefix = "route"
        case "dns_profiles": prefix = "dns"
        case "subscriptions": prefix = "subscription"
        case "local_proxies": prefix = "proxy"
        case "rules": prefix = "rule"
        default: prefix = "item"
        }
        let id = "\(prefix)-\(UUID().uuidString.lowercased().prefix(8))"
        let firstNode = draftItems(for: "nodes").first(where: \.enabled)?.identifier ?? ""
        let firstRoute = draftItems(for: "routes").first(where: \.enabled)?.identifier ?? ""
        let firstDNS = draftItems(for: "dns_profiles").first(where: \.enabled)?.identifier ?? ""
        switch key {
        case "nodes":
            return .object(["id": .string(id), "enabled": .bool(true), "name": .string("新节点"), "type": .string("vless"), "server": .string(""), "server_port": .number(443)])
        case "routes":
            return .object(["id": .string(id), "enabled": .bool(true), "name": .string("新路由"), "kind": .string("single"), "node": .string(firstNode)])
        case "dns_profiles":
            return .object(["id": .string(id), "enabled": .bool(true), "name": .string("新 DNS Profile"), "protocol": .string("https"), "server": .string(""), "server_port": .number(443), "path": .string("/dns-query")])
        case "subscriptions":
            return .object(["id": .string(id), "enabled": .bool(true), "name": .string("新订阅"), "url": .string(""), "update_interval": .string("6h")])
        case "local_proxies":
            return .object(["id": .string(id), "enabled": .bool(true), "name": .string("新本地代理"), "protocol": .string("mixed"), "listen": .string("127.0.0.1"), "listen_port": .number(1090 + Double(itemCount(for: key)))])
        case "rules":
            return .object(["id": .string(id), "enabled": .bool(true), "name": .string("新规则"), "default": .bool(false), "dns_profile": .string(firstDNS), "route": .string(firstRoute)])
        default:
            return .object(["id": .string(id), "enabled": .bool(true)])
        }
    }

    private func draftItemDetail(key: String, object: [String: JSONValue]) -> String {
        if let server = object["server"]?.stringValue {
            let port = object["server_port"]?.numberValue.map { String(Int($0)) } ?? ""
            return port.isEmpty ? server : "\(server):\(port)"
        }
        if let listen = object["listen"]?.stringValue {
            let port = object["listen_port"]?.numberValue.map { String(Int($0)) } ?? ""
            return port.isEmpty ? listen : "\(listen):\(port)"
        }
        if let url = object["url"]?.stringValue { return url }
        if key == "routes" {
            let kind = object["kind"]?.stringValue ?? ""
            if kind == "direct" { return "系统直连" }
            if kind == "block" { return "拒绝连接" }
            let node = referencedTitle(key: "nodes", identifier: object["node"]?.stringValue)
            let detour = referencedTitle(key: "routes", identifier: object["detour"]?.stringValue)
            if !detour.isEmpty { return "\(detour) → \(node.isEmpty ? "未选择节点" : node)" }
            return node.isEmpty ? "未选择节点" : node
        }
        let route = object["route"]?.stringValue
        let dns = object["dns_profile"]?.stringValue
        var details: [String] = []
        if let route, !route.isEmpty {
            details.append("Route \(referencedTitle(key: "routes", identifier: route))")
        }
        if let dns, !dns.isEmpty {
            details.append("DNS \(referencedTitle(key: "dns_profiles", identifier: dns))")
        }
        return details.isEmpty ? "待配置" : details.joined(separator: " · ")
    }

    private func referencedTitle(key: String, identifier: String?) -> String {
        guard let identifier, !identifier.isEmpty else { return "" }
        guard let root = parseDraft()?.objectValue,
              case let .array(values)? = root[key],
              let object = values.compactMap(\.objectValue).first(where: {
                  $0["id"]?.stringValue == identifier
              }) else { return "未命名项目" }
        let name = object["name"]?.stringValue ?? ""
        if !name.isEmpty { return name }
        let kind = object["type"]?.stringValue
            ?? object["kind"]?.stringValue
            ?? object["protocol"]?.stringValue
            ?? (object["default"]?.boolValue == true ? "default" : "")
        return draftItemFallbackTitle(key: key, kind: kind)
    }

    private func draftItemFallbackTitle(key: String, kind: String) -> String {
        if key == "routes" {
            if kind == "direct" { return "Direct" }
            if kind == "block" { return "Block" }
            return "未命名路由"
        }
        if key == "rules", kind == "default" { return "Default" }
        switch key {
        case "nodes": return "未命名节点"
        case "dns_profiles": return "未命名 DNS Profile"
        case "local_proxies": return "未命名本地代理"
        case "rules": return "未命名规则"
        case "subscriptions": return "未命名订阅"
        default: return "未命名项目"
        }
    }
}

struct DraftItem: Identifiable {
    let id: String
    let index: Int
    let identifier: String
    let title: String
    let kind: String
    let detail: String
    let enabled: Bool
    let subscriptionOwned: Bool
}

private extension JSONEncoder {
    static let pretty: JSONEncoder = {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
        return encoder
    }()
}
