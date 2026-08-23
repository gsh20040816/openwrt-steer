// SPDX-License-Identifier: GPL-3.0-or-later

import SwiftUI

struct ContentView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        NavigationSplitView {
            List(AppPage.allCases, selection: Binding<AppPage?>(
                get: { model.selectedPage },
                set: { if let value = $0 { model.selectedPage = value } }
            )) { page in
                Label(page.rawValue, systemImage: page.systemImage)
                    .tag(page)
            }
            .navigationTitle("Steer")
        } detail: {
            PageView(model: model)
        }
    }
}

struct PageView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        Group {
            switch model.selectedPage {
            case .overview: OverviewView(model: model)
            case .configuration: ConfigurationView(model: model)
            case .nodes: DraftCollectionView(model: model, title: "Nodes", key: "nodes", symbol: "point.3.connected.trianglepath.dotted", hint: "节点协议字段先以 canonical JSON 保存，新增/删除和顺序已接通。")
            case .routes: DraftCollectionView(model: model, title: "Routes", key: "routes", symbol: "arrow.triangle.branch", hint: "Route、detour chain 和 Direct/Block 目标。")
            case .dns: DraftCollectionView(model: model, title: "DNS Profiles", key: "dns_profiles", symbol: "network", hint: "Bootstrap、DNS Profile、缓存和上游协议。")
            case .rules: DraftCollectionView(model: model, title: "Rules", key: "rules", symbol: "list.number", hint: "first-match 顺序；Default 仍由 Go Validate 固定约束。")
            case .subscriptions: DraftCollectionView(model: model, title: "Subscriptions", key: "subscriptions", symbol: "arrow.down.circle", hint: "订阅刷新、stable ID 和 stale cleanup。")
            case .proxies: DraftCollectionView(model: model, title: "Local Proxies", key: "local_proxies", symbol: "rectangle.connected.to.line.below", hint: "本地 SOCKS/HTTP/Mixed listeners。")
            case .diagnostics: DiagnosticsView(model: model)
            case .settings: SettingsView(model: model)
            }
        }
        .padding(24)
        .navigationTitle(model.selectedPage.rawValue)
    }
}

struct OverviewView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text(model.runtime.healthy ? "Connected" : "Disabled")
                        .font(.largeTitle.weight(.semibold))
                    Text(model.runtime.generationID.isEmpty ? "No active generation" : model.runtime.generationID)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Toggle("Enabled", isOn: Binding(get: { model.runtime.healthy }, set: { _ in model.toggleEnabled() }))
                    .toggleStyle(.switch)
            }
            HStack(spacing: 12) {
                Button("Validate") { model.validate() }
                    .buttonStyle(.borderedProminent)
                Button("Apply") { model.apply() }
                    .buttonStyle(.bordered)
                    .disabled(model.isDirty == false)
            }
            GroupBox("Runtime") {
                LabeledContent("Healthy", value: model.runtime.healthy ? "Yes" : "No")
                LabeledContent("Intent digest", value: model.runtime.intentDigest.isEmpty ? "—" : model.runtime.intentDigest)
                LabeledContent("Last message", value: model.message.isEmpty ? "—" : model.message)
            }
            Spacer()
        }
    }
}

struct ConfigurationView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Canonical JSON draft")
                    .font(.headline)
                Spacer()
                Button("Load draft") { model.loadDraft() }
                Button("Save draft") { model.saveDraft() }
                Button("Validate") { model.validate() }
            }
            TextEditor(text: Binding(get: { model.rawJSON }, set: { model.rawJSON = $0; model.markDirty() }))
                .font(.system(.body, design: .monospaced))
                .border(.quaternary)
            Text("字段级表单将逐页替换这里；Raw JSON 始终保留为导入导出和高级逃生口。")
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
    }
}

struct DraftCollectionView: View {
    @ObservedObject var model: AppModel
    let title: String
    let key: String
    let symbol: String
    let hint: String

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label(title, systemImage: symbol)
                    .font(.title2.weight(.semibold))
                Spacer()
                Button("Add") { model.appendDraftItem(to: key) }
                    .buttonStyle(.borderedProminent)
            }
            Text(hint)
                .foregroundStyle(.secondary)
            List {
                ForEach(model.draftItems(for: key)) { item in
                    HStack {
                        VStack(alignment: .leading, spacing: 3) {
                            Text(item.identifier)
                                .font(.headline)
                            if !item.summary.isEmpty {
                                Text(item.summary)
                                    .font(.footnote)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        Spacer()
                        Button(role: .destructive) {
                            model.removeDraftItem(from: key, at: item.index)
                        } label: {
                            Image(systemName: "trash")
                        }
                        .buttonStyle(.borderless)
                    }
                }
                .onMove { source, destination in
                    model.moveDraftItem(in: key, from: source, to: destination)
                }
            }
            .overlay {
                if model.draftItems(for: key).isEmpty {
                    Text("当前 draft 没有 (title)")
                        .foregroundStyle(.secondary)
                }
            }
        }
    }
}

struct DiagnosticsView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        List {
            Section("Validation") {
                if let validation = model.validation {
                    LabeledContent("OK", value: validation.ok ? "Yes" : "No")
                    ForEach(validation.errors) { issue in
                        Label(issue.message, systemImage: "xmark.octagon")
                    }
                    ForEach(validation.warnings) { issue in
                        Label(issue.message, systemImage: "exclamationmark.triangle")
                    }
                } else {
                    Text("尚未校验")
                        .foregroundStyle(.secondary)
                }
            }
            Section("Provider") {
                LabeledContent("Packet Tunnel", value: "NetworkExtension pending")
                LabeledContent("DNS Proxy", value: "NetworkExtension pending")
                LabeledContent("Generation", value: model.runtime.generationID.isEmpty ? "—" : model.runtime.generationID)
            }
        }
    }
}

struct SettingsView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        Form {
            Section("Lifecycle") {
                Toggle("Start at login", isOn: .constant(false))
                Toggle("Reconnect after network change", isOn: .constant(true))
            }
            Section("Storage") {
                LabeledContent("App Group", value: "Configured by signed target")
                LabeledContent("Geo data", value: "Explicit toolchain required")
            }
            Section {
                Text(model.message.isEmpty ? "—" : model.message)
                    .foregroundStyle(.secondary)
            }
        }
    }
}
