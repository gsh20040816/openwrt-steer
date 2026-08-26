// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation
import SwiftUI

struct DraftEditorTarget: Identifiable {
    let id = UUID()
    let key: String
    let index: Int?
    let title: String
    let object: [String: JSONValue]
}

struct DraftItemEditor: View {
    @ObservedObject var model: AppModel
    let target: DraftEditorTarget
    @Environment(\.dismiss) private var dismiss
    @State private var object: [String: JSONValue]
    @State private var errorMessage = ""

    init(model: AppModel, target: DraftEditorTarget) {
        self.model = model
        self.target = target
        _object = State(initialValue: target.object)
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                VStack(alignment: .leading, spacing: 3) {
                    Text(target.title)
                        .font(.title2.weight(.semibold))
                }
                Spacer()
            }
            .padding(20)
            Divider()

            Form {
                switch target.key {
                case "nodes":
                    SharedNodeDraftForm(object: $object)
                case "routes":
                    RouteDraftForm(
                        model: model,
                        object: $object,
                        editingIndex: target.index,
                        originalKind: target.object["kind"]?.stringValue ?? ""
                    )
                case "dns_profiles":
                    DNSDraftForm(object: $object)
                case "local_proxies":
                    LocalProxyDraftForm(object: $object)
                case "rules":
                    RuleDraftForm(model: model, object: $object, editingIndex: target.index)
                case "subscriptions":
                    SubscriptionDraftForm(object: $object)
                default:
                    Section {
                        Label("该对象没有可用的原生编辑器。", systemImage: "exclamationmark.triangle")
                    }
                }
            }
            .formStyle(.grouped)

            Divider()
            HStack(spacing: 12) {
                if !errorMessage.isEmpty {
                    Label(errorMessage, systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(.red)
                        .font(.callout)
                }
                Spacer()
                Button("取消") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button("保存到工作副本") { save() }
                    .buttonStyle(.borderedProminent)
                    .keyboardShortcut(.defaultAction)
            }
            .padding(16)
        }
        .frame(minWidth: 760, minHeight: 680)
    }

    private func save() {
        let value = normalizedObject(object, key: target.key)
        if let error = validate(value) {
            errorMessage = error
            return
        }
        let saved: Bool
        if let index = target.index {
            saved = model.replaceDraftItem(in: target.key, at: index, object: value)
        } else {
            saved = model.appendDraftItem(to: target.key, object: value)
        }
        if saved {
            dismiss()
        } else {
            errorMessage = "无法更新工作副本"
        }
    }

    private func validate(_ value: [String: JSONValue]) -> String? {
        let identifier = draftString(value, "id").trimmingCharacters(in: .whitespacesAndNewlines)
        if identifier.isEmpty || identifier.contains(where: { $0.isWhitespace }) {
            return "内部标识无效；请取消后重新创建"
        }
        if model.draftItems(for: target.key).contains(where: {
            $0.identifier == identifier && $0.index != target.index
        }) {
            return "内部标识冲突；请取消后重新创建"
        }

        switch target.key {
        case "nodes":
            let type = draftString(value, "type")
            if type.isEmpty { return "请选择节点协议" }
            for field in SteerUISpec.nodeFields(for: type) where field.isRequired(for: type) {
                let missing: Bool
                switch field.control {
                case "integer", "select-integer": missing = draftInt(value, field.key) == 0
                case "string-list": missing = draftStringList(value, field.key).isEmpty
                default: missing = draftString(value, field.key).isEmpty
                }
                if missing { return "\(field.label)不能为空" }
            }
            if type != "tor" && !validPort(draftInt(value, "server_port")) { return "服务器端口必须是 1…65535" }
            if type == "ssh", draftString(value, "password").isEmpty && draftString(value, "private_key").isEmpty {
                return "SSH 需要密码或私钥"
            }
            if type == "shadowtls", draftInt(value, "version") >= 2, draftString(value, "password").isEmpty {
                return "ShadowTLS v2/v3 密码不能为空"
            }
        case "routes":
            let kind = draftString(value, "kind")
            if !["direct", "block", "single"].contains(kind) { return "请选择路由类型" }
            if kind == "single" && draftString(value, "node").isEmpty { return "Single Route 必须选择节点" }
        case "dns_profiles":
            let proto = draftString(value, "protocol")
            if draftString(value, "server").isEmpty { return "DNS 服务器不能为空" }
            if !validPort(draftInt(value, "server_port")) { return "DNS 端口必须是 1…65535" }
            if ["tls", "https", "quic", "h3"].contains(proto), draftString(value, "tls_server_name").isEmpty {
                return "加密 DNS 需要 TLS 服务器名"
            }
        case "local_proxies":
            if draftString(value, "listen").isEmpty { return "监听地址不能为空" }
            if !validPort(draftInt(value, "listen_port")) { return "监听端口必须是 1…65535" }
            let username = draftString(value, "username")
            let password = draftString(value, "password")
            if username.isEmpty != password.isEmpty { return "用户名和密码必须同时填写" }
        case "rules":
            if draftString(value, "route").isEmpty { return "请选择 Route" }
            if draftString(value, "dns_profile").isEmpty { return "请选择 DNS Profile" }
            if !draftBool(value, "default") && !ruleHasMatch(value) { return "非 Default 规则至少需要一个匹配条件" }
        case "subscriptions":
            let rawURL = draftString(value, "url")
            guard let url = URLComponents(string: rawURL),
                  ["http", "https"].contains(url.scheme?.lowercased() ?? ""),
                  url.host?.isEmpty == false else { return "订阅 URL 必须是完整的 HTTP 或 HTTPS 地址" }
        default:
            break
        }
        return nil
    }
}

struct NodeImportSheet: View {
    @ObservedObject var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var document = ""
    @State private var errorMessage = ""

    var body: some View {
        VStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 4) {
                Text("导入节点")
                    .font(.title2.weight(.semibold))
                Text("每行一个分享链接，也支持 Base64 包装的订阅内容。")
                    .foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(20)
            Divider()
            Form {
                Section("分享链接") {
                    TextEditor(text: $document)
                        .font(.system(.body, design: .monospaced))
                        .frame(minHeight: 260)
                    Text("支持 VLESS、VMess、Trojan、Hysteria、TUIC、Shadowsocks、SOCKS、HTTP、SSH 等由 Steer 后端解析的格式。")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .formStyle(.grouped)
            Divider()
            HStack {
                if !errorMessage.isEmpty {
                    Label(errorMessage, systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(.red)
                }
                Spacer()
                Button("取消") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button("导入到工作副本") {
                    Task {
                        if await model.importNodes(document) {
                            dismiss()
                        } else {
                            errorMessage = model.message
                        }
                    }
                }
                .buttonStyle(.borderedProminent)
                .keyboardShortcut(.defaultAction)
                .disabled(document.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || model.isBusy)
            }
            .padding(16)
        }
        .frame(minWidth: 680, minHeight: 500)
    }
}

// The controls remain native SwiftUI, while their field matrix, enum values,
// conditional visibility and allowed-option set come from the generated Go
// UI contract.
private struct SharedNodeDraftForm: View {
    @Binding var object: [String: JSONValue]

    private var type: String { draftString(object, "type").isEmpty ? "vless" : draftString(object, "type") }

    var body: some View {
        Section("基本信息") {
            Picker("协议", selection: stringBinding($object, "type", required: true, defaultValue: "vless")) {
                ForEach(SteerUISpec.contract.nodeTypes) { option in
                    Text(option.label).tag(option.value)
                }
            }
            fields(section: "general")
        }
        if hasVisibleFields(section: "protocol") {
            Section("认证与协议") { fields(section: "protocol") }
        }
        if hasVisibleFields(section: "transport") {
            Section("传输") { fields(section: "transport") }
        }
        if hasVisibleFields(section: "tls") {
            Section("TLS / REALITY") { fields(section: "tls") }
        }
        if hasVisibleFields(section: "advanced") {
            Section("高级协议参数") { fields(section: "advanced") }
        }
    }

    @ViewBuilder
    private func fields(section: String) -> some View {
        ForEach(SteerUISpec.nodeFields(for: type, section: section).filter(visible)) { field in
            fieldControl(field)
        }
    }

    @ViewBuilder
    private func fieldControl(_ field: UIFieldSpec) -> some View {
        let label = localizedLabel(field)
        switch field.control {
        case "boolean":
            Toggle(label, isOn: boolBinding($object, field.key, defaultValue: field.defaultValue?.boolValue ?? false))
        case "integer":
            TextField(label, value: intBinding($object, field.key, defaultValue: Int(field.defaultValue?.numberValue ?? 0)), format: .number)
        case "select":
            Picker(label, selection: stringBinding($object, field.key, defaultValue: field.defaultValue?.stringValue ?? "")) {
                ForEach(field.options) { option in Text(option.label).tag(option.value) }
            }
        case "select-integer":
            Picker(label, selection: intBinding($object, field.key, defaultValue: Int(field.defaultValue?.numberValue ?? 0))) {
                ForEach(field.options) { option in Text(option.label).tag(Int(option.value) ?? 0) }
            }
        case "string-list":
            TextField(label, text: stringListBinding($object, field.key), prompt: prompt(field))
        case "password" where field.multiline:
            LabeledContent(label) {
                TextEditor(text: stringBinding($object, field.key))
                    .font(.system(.body, design: .monospaced))
                    .frame(minHeight: 76)
            }
        case "password":
            SecureField(label, text: stringBinding($object, field.key, required: field.isRequired(for: type)))
        default:
            TextField(label, text: stringBinding($object, field.key, required: field.isRequired(for: type)), prompt: prompt(field))
        }
    }

    private func visible(_ field: UIFieldSpec) -> Bool {
        guard let condition = field.when else { return true }
        let current = draftString(object, condition.field)
        if !current.isEmpty { return condition.values.contains(current) }
        let fallback = SteerUISpec.nodeFields(for: type).first { $0.key == condition.field }?.defaultValue?.stringValue ?? ""
        return condition.values.contains(fallback)
    }

    private func hasVisibleFields(section: String) -> Bool {
        SteerUISpec.nodeFields(for: type, section: section).contains(where: visible)
    }

    private func prompt(_ field: UIFieldSpec) -> Text? {
        field.placeholder.isEmpty ? nil : Text(field.placeholder)
    }

    private func localizedLabel(_ field: UIFieldSpec) -> String {
        let labels = [
            "enabled": "启用节点", "name": "名称", "server": "服务器", "server_port": "服务器端口",
            "username": "用户名", "password": "密码", "method": "加密方法", "plugin": "插件", "plugin_options": "插件参数",
            "security": "Security", "alter_id": "Alter ID", "network": "Network", "packet_encoding": "Packet encoding",
            "flow": "Flow", "transport": "传输类型", "transport_path": "路径", "transport_host": "Host",
            "service_name": "Service name", "server_ports": "端口范围", "hop_interval": "跳跃间隔", "obfs_type": "混淆",
            "obfs_password": "混淆密码", "up_mbps": "上传 Mbps", "down_mbps": "下载 Mbps", "version": "版本",
            "congestion_control": "拥塞控制", "udp_relay_mode": "UDP relay", "udp_over_stream": "UDP over stream",
            "zero_rtt_handshake": "Zero-RTT handshake", "heartbeat": "Heartbeat", "quic_congestion_control": "QUIC congestion control",
            "insecure_concurrency": "Insecure concurrency", "private_key": "Private key", "host_key": "Host key",
            "host_key_algorithms": "Host key algorithms", "executable_path": "可执行文件", "extra_args": "额外参数",
            "data_directory": "数据目录", "tls_server_name": "TLS 服务器名", "utls_fingerprint": "uTLS 指纹",
            "insecure": "跳过证书验证", "reality_public_key": "REALITY Public key", "reality_short_id": "REALITY Short ID",
        ]
        return labels[field.key] ?? field.label
    }
}

private struct RouteDraftForm: View {
    @ObservedObject var model: AppModel
    @Binding var object: [String: JSONValue]
    let editingIndex: Int?
    let originalKind: String

    private var kind: String { draftString(object, "kind") }
    private var isSystemRoute: Bool { ["direct", "block"].contains(originalKind) }

    var body: some View {
        Section("路由") {
            if originalKind == "direct" {
                LabeledContent("状态") {
                    Label("系统必需 · 始终启用", systemImage: "lock.fill")
                        .foregroundStyle(.green)
                }
            } else {
                Toggle("启用路由", isOn: boolBinding($object, "enabled", defaultValue: true))
            }
            TextField("名称", text: stringBinding($object, "name"))
            if isSystemRoute {
                LabeledContent("类型", value: kind == "direct" ? "Direct" : "Reject")
            } else {
                LabeledContent("类型", value: "Single 节点")
            }
            if isSystemRoute {
                Text("系统路由类型固定；可以修改显示名称。")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        if kind == "single" {
            Section("出口关系") {
                Picker("节点", selection: stringBinding($object, "node", required: true)) {
                    Text("请选择节点").tag("")
                    ForEach(model.draftItems(for: "nodes")) { item in
                        Text("\(item.title) · \(item.kind.uppercased())").tag(item.identifier)
                    }
                }
                Picker("前置路由 (detour)", selection: stringBinding($object, "detour")) {
                    Text("直连（无前置）").tag("")
                    ForEach(model.draftItems(for: "routes").filter {
                        $0.kind == "single" && $0.index != editingIndex
                    }) { item in
                        Text(item.title).tag(item.identifier)
                    }
                }
                Text("非空时先拨号前置路由，再连接当前节点。")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
    }
}

private struct DNSDraftForm: View {
    @Binding var object: [String: JSONValue]
    private var proto: String { draftString(object, "protocol") }

    var body: some View {
        Section("DNS Profile") {
            Toggle("启用 Profile", isOn: boolBinding($object, "enabled", defaultValue: true))
            TextField("名称", text: stringBinding($object, "name"))
            Picker("协议", selection: stringBinding($object, "protocol", required: true, defaultValue: "https")) {
                ForEach(SteerUISpec.contract.dnsProtocols) { option in
                    Text(option.label).tag(option.value)
                }
            }
            TextField("服务器", text: stringBinding($object, "server", required: true), prompt: Text("1.1.1.1"))
            TextField("端口", value: intBinding($object, "server_port", defaultValue: 443), format: .number)
        }
        if ["tls", "https", "quic", "h3"].contains(proto) {
            Section("加密传输") {
                TextField("TLS 服务器名", text: stringBinding($object, "tls_server_name", required: true), prompt: Text("one.one.one.one"))
                if ["https", "h3"].contains(proto) {
                    TextField("HTTP 路径", text: stringBinding($object, "path"), prompt: Text("/dns-query"))
                }
                Toggle("跳过证书验证", isOn: boolBinding($object, "insecure"))
            }
        }
    }
}

private struct LocalProxyDraftForm: View {
    @Binding var object: [String: JSONValue]

    var body: some View {
        Section("本地入口") {
            Toggle("启用入口", isOn: boolBinding($object, "enabled", defaultValue: true))
            TextField("名称", text: stringBinding($object, "name"))
            Picker("协议", selection: stringBinding($object, "protocol", required: true, defaultValue: "mixed")) {
                ForEach(SteerUISpec.contract.localProxyProtocols) { option in
                    Text(option.label).tag(option.value)
                }
            }
            TextField("监听地址", text: stringBinding($object, "listen", required: true), prompt: Text("127.0.0.1"))
            TextField("监听端口", value: intBinding($object, "listen_port", defaultValue: 1090), format: .number)
        }
        Section("认证（可选）") {
            TextField("用户名", text: stringBinding($object, "username"))
            SecureField("密码", text: stringBinding($object, "password"))
            Text("非 loopback 监听必须同时设置用户名和密码。")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }
}

private struct RuleDraftForm: View {
    @ObservedObject var model: AppModel
    @Binding var object: [String: JSONValue]
    let editingIndex: Int?

    private var isDefault: Bool { draftBool(object, "default") }
    private var hasOtherDefault: Bool {
        model.draftItems(for: "rules").contains {
            $0.kind == "default" && $0.index != editingIndex
        }
    }

    var body: some View {
        Section("规则") {
            Toggle("启用规则", isOn: boolBinding($object, "enabled", defaultValue: true))
            TextField("名称", text: stringBinding($object, "name"))
            Toggle("Default 规则", isOn: defaultRuleBinding)
                .disabled(hasOtherDefault && !isDefault)
            if hasOtherDefault && !isDefault {
                Text("配置中已经存在 Default 规则。")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }

        Section("决策") {
            Picker("Route", selection: stringBinding($object, "route", required: true)) {
                Text("请选择 Route").tag("")
                ForEach(model.draftItems(for: "routes")) { item in
                    Text(item.title).tag(item.identifier)
                }
            }
            Picker("DNS Profile", selection: stringBinding($object, "dns_profile", required: true)) {
                Text("请选择 DNS Profile").tag("")
                ForEach(model.draftItems(for: "dns_profiles")) { item in
                    Text(item.title).tag(item.identifier)
                }
            }
        }

        if !isDefault {
            Section("目标匹配") {
                TextField("Domain match", text: stringListBinding($object, "domain_match"), prompt: Text("domain:example.com, geosite:cn"))
                if !model.geositeNames.isEmpty {
                    Menu("添加 GeoSite") {
                        ForEach(model.geositeNames.prefix(100), id: \.self) { name in
                            Button(name) { appendMatch("domain_match", value: "geosite:\(name)") }
                        }
                    }
                }
                TextField("IP match", text: stringListBinding($object, "ip_match"), prompt: Text("10.0.0.0/8, geoip:cn"))
                if !model.geoipNames.isEmpty {
                    Menu("添加 GeoIP") {
                        ForEach(model.geoipNames.prefix(100), id: \.self) { name in
                            Button(name) { appendMatch("ip_match", value: "geoip:\(name)") }
                        }
                    }
                }
                TextField("目标端口", text: intListBinding($object, "port"), prompt: Text("443, 8443"))
            }
            Section("来源匹配") {
                TextField("Source IP CIDR", text: stringListBinding($object, "source_ip_cidr"), prompt: Text("192.168.1.0/24"))
                if model.draftItems(for: "local_proxies").isEmpty {
                    LabeledContent("本地入口", value: "无")
                } else {
                    DisclosureGroup("本地入口") {
                        ForEach(model.draftItems(for: "local_proxies")) { item in
                            Toggle(item.title, isOn: membershipBinding($object, "inbound", item.identifier))
                        }
                    }
                }
            }
            Section("连接条件") {
                ForEach(SteerUISpec.contract.ruleNetworks) { option in
                    Toggle(option.label, isOn: membershipBinding($object, "network", option.value))
                }
                DisclosureGroup("嗅探协议") {
                    ForEach(SteerUISpec.contract.ruleProtocols) { option in
                        Toggle(option.label, isOn: membershipBinding($object, "protocol", option.value))
                    }
                }
            }
        } else {
            Section {
                Text("Default 不包含匹配条件，并固定在规则列表最后。")
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var defaultRuleBinding: Binding<Bool> {
        Binding(
            get: { draftBool(object, "default") },
            set: { enabled in
                var copy = object
                copy["default"] = .bool(enabled)
                if enabled {
                    for key in ruleMatchKeys { copy.removeValue(forKey: key) }
                }
                object = copy
            }
        )
    }

    private func appendMatch(_ key: String, value: String) {
        var values = draftStringList(object, key)
        if !values.contains(value) { values.append(value) }
        object[key] = .array(values.map(JSONValue.string))
    }

}

private struct SubscriptionDraftForm: View {
    @Binding var object: [String: JSONValue]

    var body: some View {
        Section("订阅源") {
            Toggle("启用订阅", isOn: boolBinding($object, "enabled", defaultValue: true))
            TextField("名称", text: stringBinding($object, "name"))
            TextField("URL", text: stringBinding($object, "url", required: true), prompt: Text("https://example.com/subscription"))
            TextField("更新间隔", text: stringBinding($object, "update_interval"), prompt: Text(SteerUISpec.contract.subscriptionUpdateIntervalDefault))
            Text("订阅刷新由平台后端执行；订阅生成的节点在节点页保持只读。")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }
}

private func stringBinding(
    _ object: Binding<[String: JSONValue]>,
    _ key: String,
    required: Bool = false,
    defaultValue: String = ""
) -> Binding<String> {
    Binding(
        get: {
            let value = draftString(object.wrappedValue, key)
            return value.isEmpty ? defaultValue : value
        },
        set: { value in
            var copy = object.wrappedValue
            if value.isEmpty && !required {
                copy.removeValue(forKey: key)
            } else {
                copy[key] = .string(value)
            }
            object.wrappedValue = copy
        }
    )
}

private func boolBinding(
    _ object: Binding<[String: JSONValue]>,
    _ key: String,
    defaultValue: Bool = false
) -> Binding<Bool> {
    Binding(
        get: { object.wrappedValue[key]?.boolValue ?? defaultValue },
        set: { value in
            var copy = object.wrappedValue
            copy[key] = .bool(value)
            object.wrappedValue = copy
        }
    )
}

private func intBinding(
    _ object: Binding<[String: JSONValue]>,
    _ key: String,
    defaultValue: Int = 0
) -> Binding<Int> {
    Binding(
        get: { Int(object.wrappedValue[key]?.numberValue ?? Double(defaultValue)) },
        set: { value in
            var copy = object.wrappedValue
            if value == 0 && defaultValue == 0 {
                copy.removeValue(forKey: key)
            } else {
                copy[key] = .number(Double(value))
            }
            object.wrappedValue = copy
        }
    )
}

private func stringListBinding(_ object: Binding<[String: JSONValue]>, _ key: String) -> Binding<String> {
    Binding(
        get: { draftStringList(object.wrappedValue, key).joined(separator: ", ") },
        set: { raw in
            let values = splitList(raw).map(JSONValue.string)
            var copy = object.wrappedValue
            if values.isEmpty { copy.removeValue(forKey: key) } else { copy[key] = .array(values) }
            object.wrappedValue = copy
        }
    )
}

private func intListBinding(_ object: Binding<[String: JSONValue]>, _ key: String) -> Binding<String> {
    Binding(
        get: { draftIntList(object.wrappedValue, key).map(String.init).joined(separator: ", ") },
        set: { raw in
            let values = splitList(raw).compactMap(Int.init).map { JSONValue.number(Double($0)) }
            var copy = object.wrappedValue
            if values.isEmpty { copy.removeValue(forKey: key) } else { copy[key] = .array(values) }
            object.wrappedValue = copy
        }
    )
}

private func membershipBinding(
    _ object: Binding<[String: JSONValue]>,
    _ key: String,
    _ value: String
) -> Binding<Bool> {
    Binding(
        get: { draftStringList(object.wrappedValue, key).contains(value) },
        set: { enabled in
            var values = draftStringList(object.wrappedValue, key)
            if enabled && !values.contains(value) { values.append(value) }
            if !enabled { values.removeAll { $0 == value } }
            var copy = object.wrappedValue
            if values.isEmpty {
                copy.removeValue(forKey: key)
            } else {
                copy[key] = .array(values.map(JSONValue.string))
            }
            object.wrappedValue = copy
        }
    )
}

private func draftString(_ object: [String: JSONValue], _ key: String) -> String {
    object[key]?.stringValue ?? ""
}

private func draftBool(_ object: [String: JSONValue], _ key: String) -> Bool {
    object[key]?.boolValue ?? false
}

private func draftInt(_ object: [String: JSONValue], _ key: String) -> Int {
    Int(object[key]?.numberValue ?? 0)
}

private func draftStringList(_ object: [String: JSONValue], _ key: String) -> [String] {
    object[key]?.arrayValue?.compactMap(\.stringValue) ?? []
}

private func draftIntList(_ object: [String: JSONValue], _ key: String) -> [Int] {
    object[key]?.arrayValue?.compactMap(\.numberValue).map(Int.init) ?? []
}

private func splitList(_ raw: String) -> [String] {
    raw.split(whereSeparator: { $0 == "," || $0 == "\n" })
        .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
        .filter { !$0.isEmpty }
}

private func validPort(_ value: Int) -> Bool {
    (1...65_535).contains(value)
}

private let ruleMatchKeys = [
    "inbound", "domain_match", "ip_match", "source_ip_cidr", "source_mac_address",
    "network", "protocol", "port",
]

private func ruleHasMatch(_ object: [String: JSONValue]) -> Bool {
    ruleMatchKeys.contains { object[$0]?.arrayValue?.isEmpty == false }
}

private func normalizedObject(_ source: [String: JSONValue], key: String) -> [String: JSONValue] {
    var object = source
    object["id"] = .string(draftString(object, "id").trimmingCharacters(in: .whitespacesAndNewlines))

    let requiredStrings: Set<String>
    switch key {
    case "nodes": requiredStrings = ["id", "type", "server"]
    case "routes": requiredStrings = ["id", "kind"]
    case "dns_profiles": requiredStrings = ["id", "protocol", "server"]
    case "local_proxies": requiredStrings = ["id", "protocol", "listen"]
    case "rules": requiredStrings = ["id", "dns_profile", "route"]
    case "subscriptions": requiredStrings = ["id", "url"]
    default: requiredStrings = ["id"]
    }

    for field in Array(object.keys) {
        guard let value = object[field] else { continue }
        if value.stringValue?.isEmpty == true && !requiredStrings.contains(field) {
            object.removeValue(forKey: field)
        }
        if value.arrayValue?.isEmpty == true {
            object.removeValue(forKey: field)
        }
    }

    if key == "routes", draftString(object, "kind") != "single" {
        object.removeValue(forKey: "node")
        object.removeValue(forKey: "detour")
    }
    if key == "nodes", draftString(object, "type") == "tor" {
        object.removeValue(forKey: "server")
        object.removeValue(forKey: "server_port")
    }
    if key == "nodes" {
        normalizeNodeOptionsFromSpec(&object)
    }
    if key == "rules", draftBool(object, "default") {
        for field in ruleMatchKeys { object.removeValue(forKey: field) }
    }
    return object
}

private func normalizeNodeOptionsFromSpec(_ object: inout [String: JSONValue]) {
    let type = draftString(object, "type")
    let allowed = Set(SteerUISpec.nodeFields(for: type).map(\.key))
    for field in nodeOptionFields where !allowed.contains(field) {
        object.removeValue(forKey: field)
    }
}

private let nodeOptionFields: Set<String> = [
    "uuid", "username", "password", "private_key", "host_key", "host_key_algorithms",
    "network", "transport", "transport_path", "transport_host", "service_name", "packet_encoding", "flow",
    "security", "alter_id", "version", "method", "plugin", "plugin_options",
    "congestion_control", "udp_relay_mode", "udp_over_stream", "zero_rtt_handshake", "heartbeat",
    "quic", "quic_congestion_control", "insecure_concurrency", "server_ports", "hop_interval",
    "obfs_type", "obfs_password", "up_mbps", "down_mbps", "executable_path", "extra_args", "data_directory",
    "tls_server_name", "insecure", "reality_public_key", "reality_short_id", "utls_fingerprint",
]
