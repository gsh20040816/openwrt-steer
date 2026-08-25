// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation
import SwiftUI

enum AppPage: String, CaseIterable, Identifiable {
    case overview = "Overview"
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
}

struct RuntimeStatus: Codable {
    var healthy = false
    var generationID = ""
    var intentDigest = ""
    var error = ""

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

indirect enum JSONValue: Codable {
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

enum BackendClientError: LocalizedError {
    case helperUnavailable
    case invalidResponse
    case validationFailed
    case processFailed(String)

    var errorDescription: String? {
        switch self {
        case .helperUnavailable:
            return "未找到 steer-macos helper；请先运行 macos/scripts/install-launchdaemon.sh。"
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
    func validate(document: String) async throws -> ValidationResult
    func loadConfiguration() async throws -> String
    func save(document: String) async throws
    func apply(document: String) async throws -> RuntimeStatus
    func status() async throws -> RuntimeStatus
}

struct HelperBackendClient: BackendClient {
    private static let installedHelperPath = "/usr/local/libexec/steer/steer-macos"
    private static let configurationPath = "/Library/Application Support/Steer/config/config.json"

    private let validationHelperURL: URL

    init(helperURL: URL? = nil) {
        validationHelperURL = helperURL
            ?? Bundle.main.url(forResource: "steer-macos", withExtension: nil)
            ?? URL(fileURLWithPath: Self.installedHelperPath)
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
        let output = try await Self.executePrivileged(Self.command([
            "/bin/cat", Self.configurationPath,
        ]))
        guard let document = String(data: output, encoding: .utf8) else {
            throw BackendClientError.invalidResponse
        }
        return document
    }

    func save(document: String) async throws {
        let validation = try await validate(document: document)
        guard validation.ok else { throw BackendClientError.validationFailed }
        try await withTemporaryDocument(document) { url in
            _ = try await Self.executePrivileged(Self.command([
                "/usr/bin/install", "-m", "0600", url.path, Self.configurationPath,
            ]))
        }
    }

    func apply(document: String) async throws -> RuntimeStatus {
        let validation = try await validate(document: document)
        guard validation.ok else { throw BackendClientError.validationFailed }
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        try requireExecutable(helper)
        return try await withTemporaryDocument(document) { url in
            let install = Self.command(["/usr/bin/install", "-m", "0600", url.path, Self.configurationPath])
            let apply = Self.command([helper.path, "apply"]) + " >/dev/null"
            let status = Self.command([helper.path, "status"])
            let output = try await Self.executePrivileged("\(install) && \(apply) && \(status)")
            guard let status = try? JSONDecoder().decode(RuntimeStatus.self, from: output) else {
                throw BackendClientError.invalidResponse
            }
            return status
        }
    }

    func status() async throws -> RuntimeStatus {
        let helper = URL(fileURLWithPath: Self.installedHelperPath)
        try requireExecutable(helper)
        let output = try await Self.executePrivileged(Self.command([helper.path, "status"]))
        guard let status = try? JSONDecoder().decode(RuntimeStatus.self, from: output) else {
            throw BackendClientError.invalidResponse
        }
        return status
    }

    private func requireExecutable(_ url: URL) throws {
        guard FileManager.default.isExecutableFile(atPath: url.path) else {
            throw BackendClientError.helperUnavailable
        }
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

    private let backend: BackendClient

    init(backend: BackendClient? = nil) {
        self.backend = backend ?? HelperBackendClient()
    }

    var draftEnabled: Bool {
        parseDraft()?.objectValue?["main"]?.objectValue?["enabled"]?.boolValue ?? false
    }

    func markDirty() {
        isDirty = true
        message = "有未应用的 draft 修改"
    }

    func loadInitialState() {
        perform(message: "正在连接 Steer 后端…") {
            self.rawJSON = try await self.backend.loadConfiguration()
            self.runtime = try await self.backend.status()
            self.isDirty = false
            self.message = self.runtime.healthy ? "Steer 运行正常" : "已连接后端，Steer 当前未运行"
        }
    }

    func refreshStatus() {
        perform(message: "正在读取运行状态…") {
            self.runtime = try await self.backend.status()
            self.message = self.runtime.healthy ? "Steer 运行正常" : "Steer 当前未运行"
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
            self.runtime = try await self.backend.apply(document: self.rawJSON)
            self.isDirty = false
            self.message = self.runtime.healthy ? "配置已应用，Steer 运行正常" : "配置已保存，Steer 已停用"
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

    func setEnabled(_ enabled: Bool) {
        guard var root = parseDraft()?.objectValue else {
            message = "当前 draft 不是 JSON object，无法修改启用状态"
            return
        }
        var main = root["main"]?.objectValue ?? [:]
        main["enabled"] = .bool(enabled)
        root["main"] = .object(main)
        writeDraft(.object(root), context: "启用状态")
    }

    func draftItems(for key: String) -> [DraftItem] {
        guard let root = parseDraft()?.objectValue,
              case let .array(values)? = root[key] else { return [] }
        return values.enumerated().map { index, value in
            let object = value.objectValue ?? [:]
            let identifier = object["id"]?.stringValue ?? "item-\(index + 1)"
            let name = object["name"]?.stringValue
            let enabled = object["enabled"]?.boolValue.map { $0 ? "enabled" : "disabled" } ?? ""
            let summary = [name, enabled].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
            return DraftItem(id: "\(key)-\(index)", index: index, identifier: identifier, summary: summary)
        }
    }

    func appendDraftItem(to key: String) {
        mutateCollection(key) { values in
            values.append(defaultItem(for: key, index: values.count + 1))
        }
    }

    func removeDraftItem(from key: String, at index: Int) {
        mutateCollection(key) { values in
            guard values.indices.contains(index) else { return }
            values.remove(at: index)
        }
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

    private func defaultItem(for key: String, index: Int) -> JSONValue {
        let id = "new-\(key)-\(index)"
        switch key {
        case "nodes":
            return .object(["id": .string(id), "enabled": .bool(false), "type": .string("socks"), "server": .string("127.0.0.1"), "server_port": .number(1080)])
        case "routes":
            return .object(["id": .string(id), "enabled": .bool(false), "kind": .string("direct")])
        case "dns_profiles":
            return .object(["id": .string(id), "enabled": .bool(false), "protocol": .string("udp"), "server": .string("1.1.1.1"), "server_port": .number(53)])
        case "subscriptions":
            return .object(["id": .string(id), "enabled": .bool(false), "url": .string("https://example.invalid/subscription")])
        case "local_proxies":
            return .object(["id": .string(id), "enabled": .bool(false), "protocol": .string("mixed"), "listen": .string("127.0.0.1"), "listen_port": .number(1090 + Double(index))])
        case "rules":
            return .object(["id": .string(id), "enabled": .bool(false), "default": .bool(false), "dns_profile": .string(""), "route": .string("")])
        default:
            return .object(["id": .string(id), "enabled": .bool(false)])
        }
    }
}

struct DraftItem: Identifiable {
    let id: String
    let index: Int
    let identifier: String
    let summary: String
}

private extension JSONEncoder {
    static let pretty: JSONEncoder = {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        return encoder
    }()
}
