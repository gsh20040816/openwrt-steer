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
                    NodeDraftForm(object: $object)
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
            if type != "tor" {
                if draftString(value, "server").isEmpty { return "服务器不能为空" }
                if !validPort(draftInt(value, "server_port")) { return "服务器端口必须是 1…65535" }
            }
            let required: [(String, String)]
            switch type {
            case "vmess", "vless", "tuic": required = [("uuid", "UUID")]
            case "shadowsocks": required = [("method", "加密方法"), ("password", "密码")]
            case "hysteria", "hysteria2", "trojan", "anytls", "naive": required = [("password", "密码")]
            case "ssh":
                if draftString(value, "username").isEmpty { return "SSH 用户名不能为空" }
                if draftString(value, "password").isEmpty && draftString(value, "private_key").isEmpty {
                    return "SSH 需要密码或私钥"
                }
                required = []
            default: required = []
            }
            if let missing = required.first(where: { draftString(value, $0.0).isEmpty }) {
                return "\(missing.1)不能为空"
            }
            if ["hysteria", "hysteria2", "trojan", "shadowtls", "tuic", "anytls", "naive"].contains(type),
               draftString(value, "tls_server_name").isEmpty {
                return "该协议需要 TLS 服务器名"
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

private struct NodeDraftForm: View {
    @Binding var object: [String: JSONValue]

    private var type: String { draftString(object, "type") }
    private var transport: String { draftString(object, "transport") }
    private var supportsTLS: Bool {
        ["http", "vmess", "hysteria", "vless", "hysteria2", "trojan", "shadowtls", "tuic", "anytls", "naive"].contains(type)
    }
    private var supportsTransport: Bool { ["vmess", "vless", "trojan"].contains(type) }

    var body: some View {
        Section("基本信息") {
            Toggle("启用节点", isOn: boolBinding($object, "enabled", defaultValue: true))
            TextField("名称", text: stringBinding($object, "name"), prompt: Text("例如 Tokyo Edge"))
            Picker("协议", selection: stringBinding($object, "type", required: true, defaultValue: "vless")) {
                ForEach(Self.nodeTypes, id: \.value) { option in
                    Text(option.label).tag(option.value)
                }
            }
            if type != "tor" {
                TextField("服务器", text: stringBinding($object, "server", required: true), prompt: Text("example.com 或 IP"))
                TextField("服务器端口", value: intBinding($object, "server_port", defaultValue: 443), format: .number)
            }
        }

        Section("认证与协议") {
            protocolFields
        }

        if supportsTransport {
            Section("传输") {
                Picker("传输类型", selection: stringBinding($object, "transport", defaultValue: "tcp")) {
                    Text("TCP / Raw").tag("tcp")
                    Text("WebSocket").tag("ws")
                    Text("gRPC").tag("grpc")
                    Text("HTTP").tag("http")
                    Text("QUIC").tag("quic")
                }
                if ["ws", "http"].contains(transport) {
                    TextField("路径", text: stringBinding($object, "transport_path"), prompt: Text("/path"))
                    TextField("Host", text: stringBinding($object, "transport_host"), prompt: Text("example.com"))
                }
                if transport == "grpc" {
                    TextField("Service name", text: stringBinding($object, "service_name"))
                }
            }
        }

        if supportsTLS {
            Section("TLS") {
                TextField("TLS 服务器名", text: stringBinding($object, "tls_server_name"), prompt: Text("server.example.com"))
                Picker("uTLS 指纹", selection: stringBinding($object, "utls_fingerprint")) {
                    Text("系统默认").tag("")
                    ForEach(["chrome", "firefox", "safari", "edge", "random"], id: \.self) { Text($0).tag($0) }
                }
                Toggle("跳过证书验证", isOn: boolBinding($object, "insecure"))
                if type == "vless" {
                    DisclosureGroup("Reality") {
                        TextField("Public key", text: stringBinding($object, "reality_public_key"))
                        TextField("Short ID", text: stringBinding($object, "reality_short_id"))
                    }
                }
            }
        }

        Section("高级协议参数") {
            DisclosureGroup("可选字段") {
                advancedProtocolFields
            }
        }
    }

    @ViewBuilder
    private var protocolFields: some View {
        switch type {
        case "socks", "http":
            TextField("用户名", text: stringBinding($object, "username"))
            SecureField("密码", text: stringBinding($object, "password"))
        case "shadowsocks":
            TextField("加密方法", text: stringBinding($object, "method", required: true), prompt: Text("2022-blake3-aes-128-gcm"))
            SecureField("密码", text: stringBinding($object, "password", required: true))
            Picker("插件", selection: stringBinding($object, "plugin")) {
                Text("无").tag("")
                Text("obfs-local").tag("obfs-local")
                Text("v2ray-plugin").tag("v2ray-plugin")
            }
            TextField("插件参数", text: stringBinding($object, "plugin_options"))
        case "vmess":
            TextField("UUID", text: stringBinding($object, "uuid", required: true))
            Picker("Security", selection: stringBinding($object, "security", defaultValue: "auto")) {
                ForEach(["auto", "none", "zero", "aes-128-gcm", "chacha20-poly1305", "aes-128-ctr"], id: \.self) { Text($0).tag($0) }
            }
            TextField("Alter ID", value: intBinding($object, "alter_id"), format: .number)
            Picker("Network", selection: stringBinding($object, "network")) {
                Text("默认").tag("")
                Text("TCP").tag("tcp")
                Text("UDP").tag("udp")
            }
            Picker("Packet encoding", selection: stringBinding($object, "packet_encoding")) {
                Text("默认").tag("")
                Text("XUDP").tag("xudp")
                Text("PacketAddr").tag("packetaddr")
            }
        case "hysteria":
            SecureField("认证密码", text: stringBinding($object, "password", required: true))
            bandwidthFields
            TextField("端口范围", text: stringListBinding($object, "server_ports"), prompt: Text("20000:20100, 443"))
            TextField("跳跃间隔", text: stringBinding($object, "hop_interval"), prompt: Text("30s"))
            SecureField("混淆密码", text: stringBinding($object, "obfs_password"))
        case "vless":
            TextField("UUID", text: stringBinding($object, "uuid", required: true))
            Picker("Flow", selection: stringBinding($object, "flow")) {
                Text("无").tag("")
                Text("XTLS Vision").tag("xtls-rprx-vision")
            }
            Picker("Packet encoding", selection: stringBinding($object, "packet_encoding")) {
                Text("默认").tag("")
                Text("XUDP").tag("xudp")
                Text("PacketAddr").tag("packetaddr")
            }
        case "hysteria2":
            SecureField("密码", text: stringBinding($object, "password", required: true))
            TextField("端口范围", text: stringListBinding($object, "server_ports"), prompt: Text("20000:20100, 443"))
            TextField("跳跃间隔", text: stringBinding($object, "hop_interval"), prompt: Text("30s"))
            Picker("混淆", selection: stringBinding($object, "obfs_type")) {
                Text("无").tag("")
                Text("Salamander").tag("salamander")
            }
            SecureField("混淆密码", text: stringBinding($object, "obfs_password"))
            bandwidthFields
        case "trojan":
            SecureField("密码", text: stringBinding($object, "password", required: true))
        case "shadowtls":
            Picker("版本", selection: intBinding($object, "version", defaultValue: 3)) {
                Text("1").tag(1)
                Text("2").tag(2)
                Text("3").tag(3)
            }
            SecureField("密码", text: stringBinding($object, "password"))
        case "tuic":
            TextField("UUID", text: stringBinding($object, "uuid", required: true))
            SecureField("密码", text: stringBinding($object, "password", required: true))
            Picker("拥塞控制", selection: stringBinding($object, "congestion_control")) {
                Text("默认").tag("")
                ForEach(["cubic", "new_reno", "bbr"], id: \.self) { Text($0).tag($0) }
            }
            Picker("UDP relay", selection: stringBinding($object, "udp_relay_mode")) {
                Text("默认").tag("")
                Text("Native").tag("native")
                Text("QUIC").tag("quic")
            }
            Toggle("UDP over stream", isOn: boolBinding($object, "udp_over_stream"))
        case "anytls":
            SecureField("密码", text: stringBinding($object, "password", required: true))
        case "naive":
            TextField("用户名", text: stringBinding($object, "username"))
            SecureField("密码", text: stringBinding($object, "password", required: true))
        case "ssh":
            TextField("用户名", text: stringBinding($object, "username", required: true))
            SecureField("密码", text: stringBinding($object, "password"))
            LabeledContent("Private key") {
                TextEditor(text: stringBinding($object, "private_key"))
                    .font(.system(.body, design: .monospaced))
                    .frame(minHeight: 76)
            }
            TextField("Host key", text: stringBinding($object, "host_key"))
            TextField("Host key algorithms", text: stringListBinding($object, "host_key_algorithms"), prompt: Text("ssh-ed25519, rsa-sha2-512"))
        case "tor":
            TextField("可执行文件", text: stringBinding($object, "executable_path"), prompt: Text("/usr/local/bin/tor"))
            TextField("数据目录", text: stringBinding($object, "data_directory"))
            TextField("额外参数", text: stringListBinding($object, "extra_args"), prompt: Text("--SocksPort, 0"))
        default:
            Text("该协议没有必填的额外认证字段。")
                .foregroundStyle(.secondary)
        }
    }

    @ViewBuilder
    private var bandwidthFields: some View {
        TextField("上传 Mbps", value: intBinding($object, "up_mbps"), format: .number)
        TextField("下载 Mbps", value: intBinding($object, "down_mbps"), format: .number)
    }

    @ViewBuilder
    private var advancedProtocolFields: some View {
        switch type {
        case "tuic":
            Toggle("Zero-RTT handshake", isOn: boolBinding($object, "zero_rtt_handshake"))
            TextField("Heartbeat", text: stringBinding($object, "heartbeat"), prompt: Text("10s"))
        case "naive":
            Toggle("QUIC", isOn: boolBinding($object, "quic"))
            TextField("QUIC congestion control", text: stringBinding($object, "quic_congestion_control"))
            TextField("Insecure concurrency", value: intBinding($object, "insecure_concurrency"), format: .number)
        default:
            Text("当前协议没有其他高级字段。")
                .foregroundStyle(.secondary)
        }
    }

    private static let nodeTypes: [(value: String, label: String)] = [
        ("vless", "VLESS"), ("vmess", "VMess"), ("trojan", "Trojan"),
        ("hysteria2", "Hysteria2"), ("hysteria", "Hysteria"), ("tuic", "TUIC"),
        ("shadowsocks", "Shadowsocks"), ("shadowtls", "ShadowTLS"),
        ("anytls", "AnyTLS"), ("naive", "NaiveProxy"),
        ("socks", "SOCKS"), ("http", "HTTP"), ("ssh", "SSH"), ("tor", "Tor"),
    ]
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
            Toggle("启用路由", isOn: boolBinding($object, "enabled", defaultValue: true))
                .disabled(originalKind == "direct")
            TextField("名称", text: stringBinding($object, "name"))
            Picker("类型", selection: stringBinding($object, "kind", required: true, defaultValue: "single")) {
                Text("Direct").tag("direct")
                Text("Block").tag("block")
                Text("Single Route").tag("single")
            }
            .disabled(isSystemRoute)
            if isSystemRoute {
                Text("Direct 与 Block 的系统语义固定；可以修改显示名称。")
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
                Text("UDP").tag("udp")
                Text("TCP").tag("tcp")
                Text("DNS over TLS").tag("tls")
                Text("DNS over HTTPS").tag("https")
                Text("DNS over QUIC").tag("quic")
                Text("DNS over HTTP/3").tag("h3")
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
                Text("Mixed (SOCKS + HTTP)").tag("mixed")
                Text("SOCKS").tag("socks")
                Text("HTTP").tag("http")
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
                TextField("IP match", text: stringListBinding($object, "ip_match"), prompt: Text("10.0.0.0/8, geoip:cn"))
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
                Toggle("TCP", isOn: membershipBinding($object, "network", "tcp"))
                Toggle("UDP", isOn: membershipBinding($object, "network", "udp"))
                DisclosureGroup("嗅探协议") {
                    ForEach(Self.sniffedProtocols, id: \.self) { value in
                        Toggle(value.uppercased(), isOn: membershipBinding($object, "protocol", value))
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

    private static let sniffedProtocols = ["tls", "http", "quic", "dns", "stun", "bittorrent", "dtls", "ssh", "rdp", "ntp"]
}

private struct SubscriptionDraftForm: View {
    @Binding var object: [String: JSONValue]

    var body: some View {
        Section("订阅源") {
            Toggle("启用订阅", isOn: boolBinding($object, "enabled", defaultValue: true))
            TextField("名称", text: stringBinding($object, "name"))
            TextField("URL", text: stringBinding($object, "url", required: true), prompt: Text("https://example.com/subscription"))
            TextField("更新间隔", text: stringBinding($object, "update_interval"), prompt: Text("6h"))
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
        normalizeNodeOptions(&object)
    }
    if key == "rules", draftBool(object, "default") {
        for field in ruleMatchKeys { object.removeValue(forKey: field) }
    }
    return object
}

private let nodeTLSFields: Set<String> = [
    "tls_server_name", "insecure", "utls_fingerprint",
]

private let nodeTransportFields: Set<String> = [
    "transport", "transport_path", "transport_host", "service_name",
]

private let nodeOptionFields: Set<String> = [
    "uuid", "username", "password", "private_key", "host_key", "host_key_algorithms",
    "network", "transport", "transport_path", "transport_host", "service_name", "packet_encoding", "flow",
    "security", "alter_id", "version", "method", "plugin", "plugin_options",
    "congestion_control", "udp_relay_mode", "udp_over_stream", "zero_rtt_handshake", "heartbeat",
    "quic", "quic_congestion_control", "insecure_concurrency", "server_ports", "hop_interval",
    "obfs_type", "obfs_password", "up_mbps", "down_mbps", "executable_path", "extra_args", "data_directory",
    "tls_server_name", "insecure", "reality_public_key", "reality_short_id", "utls_fingerprint",
]

private func normalizeNodeOptions(_ object: inout [String: JSONValue]) {
    let type = draftString(object, "type")
    var allowed: Set<String> = []
    switch type {
    case "socks":
        allowed = ["username", "password"]
    case "http":
        allowed = ["username", "password"]
        allowed.formUnion(nodeTLSFields)
    case "shadowsocks":
        allowed = ["method", "password", "plugin", "plugin_options"]
    case "vmess":
        allowed = ["uuid", "security", "alter_id", "network", "packet_encoding"]
        allowed.formUnion(nodeTransportFields)
        allowed.formUnion(nodeTLSFields)
    case "hysteria":
        allowed = ["password", "server_ports", "hop_interval", "obfs_password", "up_mbps", "down_mbps"]
        allowed.formUnion(nodeTLSFields)
    case "vless":
        allowed = ["uuid", "flow", "packet_encoding", "reality_public_key", "reality_short_id"]
        allowed.formUnion(nodeTransportFields)
        allowed.formUnion(nodeTLSFields)
    case "hysteria2":
        allowed = ["password", "server_ports", "hop_interval", "obfs_type", "obfs_password", "up_mbps", "down_mbps"]
        allowed.formUnion(nodeTLSFields)
    case "trojan":
        allowed = ["password"]
        allowed.formUnion(nodeTransportFields)
        allowed.formUnion(nodeTLSFields)
    case "shadowtls":
        allowed = ["version", "password"]
        allowed.formUnion(nodeTLSFields)
    case "tuic":
        allowed = ["uuid", "password", "congestion_control", "udp_relay_mode", "udp_over_stream", "zero_rtt_handshake", "heartbeat"]
        allowed.formUnion(nodeTLSFields)
    case "anytls":
        allowed = ["password"]
        allowed.formUnion(nodeTLSFields)
    case "naive":
        allowed = ["username", "password", "quic", "quic_congestion_control", "insecure_concurrency"]
        allowed.formUnion(nodeTLSFields)
    case "ssh":
        allowed = ["username", "password", "private_key", "host_key", "host_key_algorithms"]
    case "tor":
        allowed = ["executable_path", "extra_args", "data_directory"]
    default:
        break
    }
    for field in nodeOptionFields where !allowed.contains(field) {
        object.removeValue(forKey: field)
    }
}
