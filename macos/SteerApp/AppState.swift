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

struct ABIError: Codable {
    let code: String
    let message: String
}

struct ABIEnvelope: Codable {
    let abiVersion: Int
    let ok: Bool
    let value: JSONValue?
    let error: ABIError?

    enum CodingKeys: String, CodingKey {
        case abiVersion = "abi_version"
        case ok
        case value
        case error
    }
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

enum CoreBridgeError: LocalizedError {
    case notConfigured
    case invalidResponse
    case processFailed(String)

    var errorDescription: String? {
        switch self {
        case .notConfigured:
            return "SteerCore bridge 尚未配置；请先把 steer-macos helper 放入 app bundle。"
        case .invalidResponse:
            return "SteerCore 返回了无法识别的 ABI envelope。"
        case let .processFailed(message):
            return message
        }
    }
}

protocol CoreBridge: Sendable {
    func validate(document: String) async throws -> ValidationResult
}

struct UnconfiguredCoreBridge: CoreBridge {
    func validate(document: String) async throws -> ValidationResult {
        throw CoreBridgeError.notConfigured
    }
}

struct ProcessCoreBridge: CoreBridge {
    let executableURL: URL

    func validate(document: String) async throws -> ValidationResult {
        let executableURL = executableURL
        return try await Task.detached {
            let temporaryURL = FileManager.default.temporaryDirectory
                .appendingPathComponent("steer-draft-\(UUID().uuidString).json")
            try Data(document.utf8).write(to: temporaryURL, options: .atomic)
            defer { try? FileManager.default.removeItem(at: temporaryURL) }

            let process = Process()
            let output = Pipe()
            process.executableURL = executableURL
            process.arguments = ["validate", "--config", temporaryURL.path]
            process.standardOutput = output
            process.standardError = Pipe()
            try process.run()
            let data = output.fileHandleForReading.readDataToEndOfFile()
            process.waitUntilExit()
            guard let result = try? JSONDecoder().decode(ValidationResult.self, from: data) else {
                throw CoreBridgeError.invalidResponse
            }
            return result
        }.value
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

    private let bridge: CoreBridge
    private let appGroupIdentifier: String

    init(bridge: CoreBridge? = nil) {
        if let bridge {
            self.bridge = bridge
        } else if let helper = Bundle.main.url(forResource: "steer-macos", withExtension: nil) {
            self.bridge = ProcessCoreBridge(executableURL: helper)
        } else {
            self.bridge = UnconfiguredCoreBridge()
        }
        self.appGroupIdentifier = Bundle.main.object(forInfoDictionaryKey: "AppGroupIdentifier") as? String ?? ""
    }

    func markDirty() {
        isDirty = true
        message = "有未应用的 draft 修改"
    }

    func validate() {
        guard !isBusy else { return }
        isBusy = true
        message = "正在校验…"
        Task {
            defer { isBusy = false }
            do {
                validation = try await bridge.validate(document: rawJSON)
                message = validation?.ok == true ? "校验通过" : "校验发现问题"
            } catch {
                message = error.localizedDescription
            }
        }
    }

    func apply() {
        guard !isBusy else { return }
        // Activation is intentionally separate from Save. The eventual
        // coordinator will prepare a generation, activate both providers,
        // wait for matching health, then publish current.json.
        message = "Apply coordinator 尚未连接 NetworkExtension。"
    }

    func saveDraft() {
        guard let container = appGroupContainer else {
            message = "App Group 尚未配置，不能保存 draft。"
            return
        }
        do {
            let directory = container.appendingPathComponent("config", isDirectory: true)
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
            try Data(rawJSON.utf8).write(to: directory.appendingPathComponent("config.json"), options: .atomic)
            isDirty = false
            message = "draft 已写入 App Group config。"
        } catch {
            message = "保存 draft 失败：\(error.localizedDescription)"
        }
    }

    func loadDraft() {
        guard let container = appGroupContainer else {
            message = "App Group 尚未配置，不能读取 draft。"
            return
        }
        do {
            let url = container.appendingPathComponent("config/config.json")
            rawJSON = try String(contentsOf: url, encoding: .utf8)
            isDirty = false
            message = "已读取 App Group config。"
        } catch {
            message = "读取 draft 失败：\(error.localizedDescription)"
        }
    }

    func toggleEnabled() {
        markDirty()
        message = "启用状态将在 Apply 时交给 NetworkExtension coordinator。"
    }

    private var appGroupContainer: URL? {
        guard !appGroupIdentifier.isEmpty else { return nil }
        return FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroupIdentifier)
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
        do {
            let data = try JSONEncoder.pretty.encode(JSONValue.object(root))
            rawJSON = String(decoding: data, as: UTF8.self)
            markDirty()
        } catch {
            message = "写回 \(key) 失败：\(error.localizedDescription)"
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
            return .object(["id": .string(id), "enabled": .bool(false), "protocol": .string("udp"), "server": .string("1.1.1.1"), "server_port": .number(53), "strategy": .string("prefer_ipv4")])
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
