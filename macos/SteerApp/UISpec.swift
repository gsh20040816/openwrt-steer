// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation

struct UIChoice: Decodable, Identifiable {
    let value: String
    let label: String
    var id: String { value }
}

struct UIDNSProtocolSpec: Decodable, Identifiable {
    let value: String
    let label: String
    let fields: [String]
    let requiredFields: [String]
    let defaultPort: Int
    var id: String { value }

    enum CodingKeys: String, CodingKey {
        case value, label, fields
        case requiredFields = "required_fields"
        case defaultPort = "default_port"
    }
}

struct UICondition: Decodable {
    let field: String
    let values: [String]
}

struct UIFieldSpec: Decodable, Identifiable {
    let key: String
    let label: String
    let control: String
    let section: String
    let types: [String]
    let requiredTypes: [String]
    let options: [UIChoice]
    let when: UICondition?
    let sensitive: Bool
    let multiline: Bool
    let placeholder: String
    let defaultValue: JSONValue?

    var id: String { "\(section):\(key)" }

    enum CodingKeys: String, CodingKey {
        case key, label, control, section, types, options, when, sensitive, multiline, placeholder
        case requiredTypes = "required_types"
        case defaultValue = "default"
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        key = try container.decode(String.self, forKey: .key)
        label = try container.decode(String.self, forKey: .label)
        control = try container.decode(String.self, forKey: .control)
        section = try container.decode(String.self, forKey: .section)
        types = try container.decodeIfPresent([String].self, forKey: .types) ?? []
        requiredTypes = try container.decodeIfPresent([String].self, forKey: .requiredTypes) ?? []
        options = try container.decodeIfPresent([UIChoice].self, forKey: .options) ?? []
        when = try container.decodeIfPresent(UICondition.self, forKey: .when)
        sensitive = try container.decodeIfPresent(Bool.self, forKey: .sensitive) ?? false
        multiline = try container.decodeIfPresent(Bool.self, forKey: .multiline) ?? false
        placeholder = try container.decodeIfPresent(String.self, forKey: .placeholder) ?? ""
        defaultValue = try container.decodeIfPresent(JSONValue.self, forKey: .defaultValue)
    }

    func isRequired(for nodeType: String) -> Bool {
        requiredTypes.contains(nodeType)
    }
}

struct UIPlatformCapabilities: Decodable {
    let rawEditor: Bool
    let sourceMAC: Bool
    let systemComponents: Bool

    enum CodingKeys: String, CodingKey {
        case rawEditor = "raw_editor"
        case sourceMAC = "source_mac"
        case systemComponents = "system_components"
    }
}

struct UINavigationItem: Decodable, Identifiable {
    let key: String
    let label: String
    var id: String { key }
}

struct UINavigationGroup: Decodable, Identifiable {
    let key: String
    let label: String
    let items: [UINavigationItem]
    var id: String { key }
}

struct UIContract: Decodable {
    let schemaVersion: Int
    let canonicalSchema: Int
    let subscriptionUpdateIntervalDefault: String
    let nodeTypes: [UIChoice]
    let nodeFields: [UIFieldSpec]
    let logLevels: [UIChoice]
    let bootstrapProtocols: [UIChoice]
    let bootstrapStrategies: [UIChoice]
    let routeKinds: [UIChoice]
    let dnsProtocols: [UIDNSProtocolSpec]
    let localProxyProtocols: [UIChoice]
    let ruleNetworks: [UIChoice]
    let ruleProtocols: [UIChoice]
    let ruleMatchFields: [String]
    let domainPrefixes: [String]
    let ipPrefixes: [String]
    let platformCapabilities: [String: UIPlatformCapabilities]
    let navigation: [UINavigationGroup]

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case canonicalSchema = "canonical_schema"
        case subscriptionUpdateIntervalDefault = "subscription_update_interval_default"
        case nodeTypes = "node_types"
        case nodeFields = "node_fields"
        case logLevels = "log_levels"
        case bootstrapProtocols = "bootstrap_protocols"
        case bootstrapStrategies = "bootstrap_strategies"
        case routeKinds = "route_kinds"
        case dnsProtocols = "dns_protocols"
        case localProxyProtocols = "local_proxy_protocols"
        case ruleNetworks = "rule_networks"
        case ruleProtocols = "rule_protocols"
        case ruleMatchFields = "rule_match_fields"
        case domainPrefixes = "domain_prefixes"
        case ipPrefixes = "ip_prefixes"
        case platformCapabilities = "platform_capabilities"
        case navigation
    }
}

enum SteerUISpec {
    static let contract: UIContract = {
        guard let data = GeneratedUISpec.document.data(using: .utf8),
              let contract = try? JSONDecoder().decode(UIContract.self, from: data),
              contract.schemaVersion == 1 else {
            fatalError("Generated Steer UI specification is invalid")
        }
        return contract
    }()

    static func nodeFields(for nodeType: String, section: String? = nil) -> [UIFieldSpec] {
        contract.nodeFields.filter { field in
            field.types.contains(nodeType) && (section == nil || field.section == section)
        }
    }

    static func dnsProtocol(_ value: String) -> UIDNSProtocolSpec? {
        contract.dnsProtocols.first { $0.value == value }
    }

    static func normalizeDNSProfile(_ object: inout [String: JSONValue]) {
        guard let protocolSpec = dnsProtocol(object["protocol"]?.stringValue ?? "") else { return }
        let allowed = Set(protocolSpec.fields)
        let conditionalFields = Set(contract.dnsProtocols.flatMap(\.fields))
        for field in conditionalFields where !allowed.contains(field) {
            object.removeValue(forKey: field)
        }
    }

    static func applyDNSProtocol(_ value: String, to object: inout [String: JSONValue]) {
        guard let next = dnsProtocol(value) else { return }
        let previous = dnsProtocol(object["protocol"]?.stringValue ?? "")
        let currentPort = Int(object["server_port"]?.numberValue ?? 0)
        object["protocol"] = .string(next.value)
        if currentPort < 1 || previous.map({ currentPort == $0.defaultPort }) == true {
            object["server_port"] = .number(Double(next.defaultPort))
        }
        normalizeDNSProfile(&object)
    }
}
