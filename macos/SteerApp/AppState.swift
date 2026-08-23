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

struct RuntimeStatus: Codable {
    var healthy = false
    var generationID = ""
    var intentDigest = ""
    var error = ""
}

struct ValidationIssue: Codable, Identifiable {
    var id: String { "\(code):\(objectID):\(option)" }
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

    init(bridge: CoreBridge = UnconfiguredCoreBridge()) {
        self.bridge = bridge
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

    func toggleEnabled() {
        markDirty()
        message = "启用状态将在 Apply 时交给 NetworkExtension coordinator。"
    }
}
