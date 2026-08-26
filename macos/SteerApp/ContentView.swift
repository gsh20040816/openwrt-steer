// SPDX-License-Identifier: GPL-3.0-or-later

import SwiftUI

struct DraftActionButtons: View {
    @ObservedObject var model: AppModel

    var body: some View {
        Button {
            model.saveDraft()
        } label: {
            Label("保存", systemImage: "square.and.arrow.down")
        }
        .disabled(!model.canSaveDraft)

        Button {
            model.applySaved()
        } label: {
            Label("Apply Saved", systemImage: "bolt")
        }
        .disabled(!model.canApplySaved)

        Button {
            model.saveAndApplyDraft()
        } label: {
            Label("保存并应用", systemImage: "bolt.fill")
        }
        .disabled(!model.canSaveAndApplyDraft)
    }
}

struct ContentView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        NavigationSplitView {
            SidebarView(model: model)
        } detail: {
            PageView(model: model)
        }
        .toolbar {
            ToolbarItemGroup(placement: .primaryAction) {
                if model.isBusy {
                    ProgressView()
                        .controlSize(.small)
                }
                Label(model.runtime.healthy ? "运行中" : "未运行",
                      systemImage: model.runtime.healthy ? "checkmark.circle.fill" : "circle")
                    .foregroundStyle(model.runtime.healthy ? .green : .secondary)
                Button { model.refreshStatus() } label: {
                    Label("刷新状态", systemImage: "arrow.clockwise")
                }
                .help("刷新 LaunchDaemon 运行状态")
                .disabled(model.isBusy || model.pendingDraftAction != nil)
                Button { model.validate() } label: {
                    Label("校验", systemImage: "checkmark.shield")
                }
                .disabled(model.isBusy || model.draftSyntaxError != nil)
                DraftActionButtons(model: model)
            }
        }
        .confirmationDialog(
            model.draftGuardTitle,
            isPresented: Binding(
                get: {
                    model.pendingDraftAction != nil && model.pendingDraftAction != .terminate
                },
                set: { isPresented in
                    guard !isPresented else { return }
                    DispatchQueue.main.async {
                        if model.pendingDraftAction != nil {
                            model.resolveDraftGuard(.cancel)
                        }
                    }
                }
            ),
            titleVisibility: .visible
        ) {
            Button("保存") { model.resolveDraftGuard(.save) }
                .disabled(!model.canSaveForPendingDraftAction)
            Button("丢弃", role: .destructive) { model.resolveDraftGuard(.discard) }
            Button("取消", role: .cancel) { model.resolveDraftGuard(.cancel) }
        } message: {
            Text(model.draftGuardExplanation)
        }
        .alert(
            "Saved revision 冲突",
            isPresented: Binding(
                get: { model.revisionConflict != nil },
                set: { _ in }
            )
        ) {
            Button("Reload Saved", role: .destructive) {
                model.reloadSavedAfterRevisionConflict()
            }
            Button("保留本地 Draft", role: .cancel) {
                model.keepLocalDraftAfterRevisionConflict()
            }
            Button("显式覆盖", role: .destructive) {
                model.overwriteAfterRevisionConflict()
            }
        } message: {
            Text(model.revisionConflictExplanation)
        }
    }
}

private struct SidebarView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        List(selection: Binding<AppPage?>(
            get: { model.selectedPage },
            set: { if let page = $0 { model.selectedPage = page } }
        )) {
            ForEach(SteerUISpec.contract.navigation) { group in
                Section(groupLabel(group.key, fallback: group.label)) {
                    ForEach(group.items) { item in
                        if let page = AppPage(contractKey: item.key) {
                            sidebarRow(page, count: count(for: page))
                        }
                    }
                }
            }
        }
        .listStyle(.sidebar)
        .navigationTitle("Steer")
        .navigationSplitViewColumnWidth(min: 200, ideal: 220, max: 280)
        .safeAreaInset(edge: .bottom) {
            HStack(spacing: 8) {
                Image(systemName: model.runtime.healthy ? "checkmark.circle.fill" : "circle")
                    .foregroundStyle(model.runtime.healthy ? .green : .secondary)
                VStack(alignment: .leading, spacing: 1) {
                    Text(model.runtime.healthy ? "运行中" : "未运行")
                        .font(.caption.weight(.medium))
                    Text(model.runtime.generationID.isEmpty ? "无 active generation" : model.runtime.generationID)
                        .font(.caption2.monospaced())
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                Spacer()
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 9)
            .background(.bar)
        }
    }

    @ViewBuilder
    private func sidebarRow(_ page: AppPage, count: Int? = nil) -> some View {
        HStack {
            Label(page.navigationLabel, systemImage: page.systemImage)
            Spacer()
            if let count {
                Text(count, format: .number)
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.tertiary)
            }
        }
        .tag(page)
    }

    private func count(for page: AppPage) -> Int? {
        switch page {
        case .nodes: return model.itemCount(for: "nodes")
        case .routes: return model.itemCount(for: "routes")
        case .dns: return model.itemCount(for: "dns_profiles")
        case .proxies: return model.itemCount(for: "local_proxies")
        case .rules: return model.itemCount(for: "rules")
        case .subscriptions: return model.itemCount(for: "subscriptions")
        default: return nil
        }
    }

    private func groupLabel(_ key: String, fallback: String) -> String {
        ["status": "状态", "configuration": "配置", "services": "服务", "advanced": "高级"][key] ?? fallback
    }
}

struct PageView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        VStack(spacing: 0) {
            PageHeader(page: model.selectedPage)
            Divider()
            switch model.selectedPage {
            case .overview:
                OverviewView(model: model)
            case .general:
                ConfigurationFormView(model: model)
            case .configuration:
                ConfigurationView(model: model)
            case .nodes:
                DraftCollectionView(model: model, descriptor: .nodes)
            case .routes:
                DraftCollectionView(model: model, descriptor: .routes)
            case .dns:
                DraftCollectionView(model: model, descriptor: .dns)
            case .rules:
                DraftCollectionView(model: model, descriptor: .rules)
            case .subscriptions:
                DraftCollectionView(model: model, descriptor: .subscriptions)
            case .proxies:
                DraftCollectionView(model: model, descriptor: .proxies)
            case .diagnostics:
                DiagnosticsView(model: model)
            case .settings:
                SystemView(model: model)
            }
        }
        .navigationTitle(model.selectedPage.navigationLabel)
    }
}

private struct PageHeader: View {
    let page: AppPage

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(page.eyebrow)
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .textCase(.uppercase)
            Text(page.navigationLabel)
                .font(.largeTitle.weight(.semibold))
            Text(page.subtitle)
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 24)
        .padding(.vertical, 18)
        .background(.bar)
    }
}

struct OverviewView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        List {
            Section {
                statusContent
            } header: {
                Label("运行状态", systemImage: "shield.lefthalf.filled")
            }
            Section {
                executionContent
            } header: {
                Label("执行模型", systemImage: "point.3.filled.connected.trianglepath.dotted")
            }
            Section {
                metricGrid
            } header: {
                Label("配置规模", systemImage: "chart.bar.xaxis")
            }
            Section {
                configurationContent
            } header: {
                Label("配置摘要", systemImage: "doc.text")
            }
        }
        .listStyle(.inset)
    }

    private var statusContent: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .center, spacing: 24) {
                VStack(alignment: .leading, spacing: 8) {
                    Label(model.runtime.healthy ? "已连接" : "未运行",
                          systemImage: model.runtime.healthy ? "checkmark.circle.fill" : "pause.circle")
                        .font(.title.weight(.semibold))
                        .foregroundStyle(model.runtime.healthy ? .green : .secondary)
                    Text("LaunchDaemon + sing-box TUN")
                        .foregroundStyle(.secondary)
                    Grid(alignment: .leading, horizontalSpacing: 24, verticalSpacing: 5) {
                        GridRow {
                            Text("Generation").foregroundStyle(.secondary)
                            Text(model.runtime.generationID.isEmpty ? "—" : model.runtime.generationID)
                                .font(.body.monospaced())
                                .textSelection(.enabled)
                        }
                        GridRow {
                            Text("Intent digest").foregroundStyle(.secondary)
                            Text(model.runtime.intentDigest.isEmpty ? "—" : model.runtime.intentDigest)
                                .font(.caption.monospaced())
                                .lineLimit(1)
                                .textSelection(.enabled)
                        }
                    }
                }
                Spacer(minLength: 20)
                VStack(alignment: .trailing, spacing: 12) {
                    Toggle("启用配置", isOn: Binding(
                        get: { model.draftEnabled },
                        set: { model.setEnabledAndApply($0) }
                    ))
                    .toggleStyle(.switch)
                    .disabled(!model.canToggleEnabled)
                    .help(model.isDirty ? "请先保存或丢弃 Draft，避免连同半成品一起部署" : "立即保存并应用启用状态")
                    ControlGroup {
                        Button("校验") { model.validate() }
                            .disabled(model.draftSyntaxError != nil)
                        DraftActionButtons(model: model)
                        Button { model.refreshStatus() } label: {
                            Image(systemName: "arrow.clockwise")
                        }
                        .help("刷新状态")
                    }
                    .disabled(model.isBusy)
                }
            }
            if !model.message.isEmpty {
                Divider()
                    .padding(.vertical, 4)
                Label(model.message, systemImage: "info.circle")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }
        }
    }

    private var executionContent: some View {
        HStack(spacing: 14) {
            pipelineStep(value: model.itemCount(for: "rules"), title: "匹配规则", subtitle: "严格 first-match", symbol: "line.3.horizontal.decrease.circle")
            pipelineArrow
            pipelineStep(value: model.itemCount(for: "dns_profiles"), title: "DNS Profile", subtitle: "独立解析路径", symbol: "network")
            pipelineArrow
            pipelineStep(value: model.itemCount(for: "routes"), title: "路由", subtitle: "Direct / Reject / Single", symbol: "arrow.triangle.branch")
            pipelineArrow
            pipelineStep(value: 1, title: "网络出口", subtitle: "Darwin utun", symbol: "globe")
        }
    }

    private var metricGrid: some View {
        Grid(horizontalSpacing: 12, verticalSpacing: 12) {
            GridRow {
                metric(value: model.itemCount(for: "nodes"), label: "节点", footnote: "\(model.enabledItemCount(for: "nodes")) 已启用", symbol: "point.3.connected.trianglepath.dotted")
                metric(value: model.itemCount(for: "routes"), label: "路由", footnote: "\(model.enabledItemCount(for: "routes")) 已启用", symbol: "arrow.triangle.branch")
                metric(value: model.itemCount(for: "dns_profiles"), label: "DNS Profile", footnote: "\(model.enabledItemCount(for: "dns_profiles")) 已启用", symbol: "network")
                metric(value: model.itemCount(for: "rules"), label: "规则", footnote: "\(model.enabledItemCount(for: "rules")) 已启用", symbol: "list.number")
            }
        }
    }

    private var configurationContent: some View {
        Grid(alignment: .leading, horizontalSpacing: 28, verticalSpacing: 10) {
            GridRow {
                LabeledContent("Schema", value: model.draftSchemaVersion == 0 ? "—" : String(model.draftSchemaVersion))
                LabeledContent("Log level", value: model.draftLogLevel)
            }
            GridRow {
                LabeledContent("DNS cache", value: model.draftDNSCacheCapacity.formatted())
                LabeledContent("工作副本", value: model.isDirty ? "有未保存修改" : "已同步")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func pipelineStep(value: Int, title: String, subtitle: String, symbol: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Label(title, systemImage: symbol)
                .font(.headline)
            Text(value, format: .number)
                .font(.title2.monospacedDigit().weight(.semibold))
            Text(subtitle)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 6)
    }

    private var pipelineArrow: some View {
        Image(systemName: "chevron.right")
            .foregroundStyle(.tertiary)
    }

    private func metric(value: Int, label: String, footnote: String, symbol: String) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            Label(label, systemImage: symbol)
                .font(.headline)
            Text(value, format: .number)
                .font(.title.monospacedDigit().weight(.semibold))
            Text(footnote)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 6)
    }
}

struct ConfigurationView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label("Schema \(model.draftSchemaVersion)", systemImage: "curlybraces")
                    .foregroundStyle(.secondary)
                if model.isDirty {
                    Label("有未保存修改", systemImage: "circle.fill")
                        .foregroundStyle(.orange)
                }
                Spacer()
                ControlGroup {
                    Button("重新载入") { model.loadDraft() }
                    Button("校验") { model.validate() }
                        .disabled(model.draftSyntaxError != nil)
                    DraftActionButtons(model: model)
                }
                .disabled(model.isBusy)
            }
            VStack(alignment: .leading, spacing: 8) {
                Label("Canonical JSON", systemImage: "doc.plaintext")
                    .font(.headline)
                Divider()
                TextEditor(text: Binding(
                    get: { model.rawJSON },
                    set: { model.updateRawDraft($0) }
                ))
                .font(.system(.body, design: .monospaced))
                .textSelection(.enabled)
                .disabled(!model.canEditDraft)
            }
            if !model.message.isEmpty {
                Label(model.message, systemImage: "info.circle")
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }
            if let syntaxError = model.draftSyntaxError {
                Label("JSON 语法错误：\(syntaxError)", systemImage: "xmark.octagon.fill")
                    .font(.callout)
                    .foregroundStyle(.red)
            }
        }
        .padding(24)
    }
}

struct DraftCollectionDescriptor {
    let key: String
    let title: String
    let emptyMessage: String
    let addLabel: String
    let symbol: String
    let ordered: Bool

    static let nodes = Self(key: "nodes", title: "节点库", emptyMessage: "尚未添加节点", addLabel: "添加节点", symbol: "point.3.connected.trianglepath.dotted", ordered: false)
    static let routes = Self(key: "routes", title: "路由", emptyMessage: "尚未添加路由", addLabel: "添加路由", symbol: "arrow.triangle.branch", ordered: false)
    static let dns = Self(key: "dns_profiles", title: "DNS Profile", emptyMessage: "尚未添加 DNS Profile", addLabel: "添加 Profile", symbol: "network", ordered: false)
    static let rules = Self(key: "rules", title: "First-Match 规则", emptyMessage: "尚未添加规则", addLabel: "添加规则", symbol: "list.number", ordered: true)
    static let subscriptions = Self(key: "subscriptions", title: "订阅", emptyMessage: "尚未添加订阅", addLabel: "添加订阅", symbol: "arrow.down.circle", ordered: false)
    static let proxies = Self(key: "local_proxies", title: "本地代理", emptyMessage: "尚未添加本地代理", addLabel: "添加入口", symbol: "rectangle.connected.to.line.below", ordered: false)
}

private struct NodeCollectionGroup: Identifiable {
    let id: String
    let label: String
    let count: Int
}

private struct SubscriptionStaleList: View {
    @ObservedObject var model: AppModel
    let status: SubscriptionRuntimeStatus

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Label("stale 节点", systemImage: "clock.badge.exclamationmark")
                    .font(.headline)
                Text(status.name ?? status.id)
                    .foregroundStyle(.secondary)
                Spacer()
                Text("\(status.stale.count) 项")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
            }
            ForEach(status.stale) { node in
                HStack(alignment: .top, spacing: 12) {
                    VStack(alignment: .leading, spacing: 3) {
                        HStack {
                            Text(node.name?.isEmpty == false ? node.name! : node.id)
                                .fontWeight(.medium)
                            Text(node.id)
                                .font(.caption.monospaced())
                                .foregroundStyle(.secondary)
                            Label("pinned-stale", systemImage: "pin.fill")
                                .font(.caption)
                                .foregroundStyle(.orange)
                        }
                        Text(node.referencedBy.isEmpty
                             ? "所属订阅 \(status.id) · 未被 Route 引用，可独立清理"
                             : "阻止原因：" + node.referencedBy.map(referenceLabel).joined(separator: "，"))
                            .font(.caption)
                            .foregroundStyle(node.referencedBy.isEmpty ? Color.secondary : Color.red)
                    }
                    Spacer()
                    Button("清理") {
                        model.cleanSubscriptionNode(subscriptionID: status.id, nodeID: node.id)
                    }
                    .disabled(model.isBusy || model.isDirty
                              || model.subscriptionOperationInProgress(status.id)
                              || !node.referencedBy.isEmpty)
                    .help(node.referencedBy.isEmpty ? "只移除这个 stale 节点；不会自动 Apply" : "仍被引用，不能清理")
                }
                .padding(.vertical, 3)
            }
        }
        .padding(14)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color(nsColor: .controlBackgroundColor)))
        .overlay(RoundedRectangle(cornerRadius: 10).stroke(.quaternary))
    }

    private func referenceLabel(_ reference: SubscriptionReference) -> String {
        let label = reference.name?.isEmpty == false ? reference.name! : reference.id
        return "\(reference.objectType) \(label)"
    }
}

private struct DefaultRuleCard: View {
    let item: DraftItem
    let detail: String
    let isBusy: Bool
    let edit: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 10) {
                Label("Default", systemImage: "pin.fill")
                    .font(.headline)
                Text(item.title)
                    .foregroundStyle(.secondary)
                Spacer()
                Label("始终启用 · 固定在最后", systemImage: "lock.fill")
                    .font(.caption.weight(.medium))
                    .foregroundStyle(.green)
                Button(action: edit) {
                    Label("编辑决策", systemImage: "square.and.pencil")
                }
                .buttonStyle(.borderless)
                .disabled(isBusy)
            }
            Text(detail)
                .font(.callout.monospaced())
                .foregroundStyle(.secondary)
            Text("第 \(item.index + 1) 条 · 只能修改显示名称、DNS Profile 与 Route")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(14)
        .background(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .fill(Color(nsColor: .controlBackgroundColor))
        )
        .overlay(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .stroke(.quaternary)
        )
    }
}

struct DraftCollectionView: View {
    @ObservedObject var model: AppModel
    let descriptor: DraftCollectionDescriptor
    @State private var selection: DraftItem.ID?
    @State private var editorTarget: DraftEditorTarget?
    @State private var pendingDeletion: DraftItem?
    @State private var actionError = ""
    @State private var nodeImportPresented = false
    @State private var selectedNodeGroup = "_manual"

    private var allItems: [DraftItem] { model.draftItems(for: descriptor.key) }
    private var activeNodeGroup: String {
        nodeGroups.contains(where: { $0.id == selectedNodeGroup }) ? selectedNodeGroup : "_manual"
    }
    private var items: [DraftItem] {
        if descriptor.key == "nodes" {
            return allItems.filter { ($0.sourceSubscription ?? "_manual") == activeNodeGroup }
        }
        if descriptor.key == "rules" {
            return allItems.filter { !isDefaultRule($0) }
        }
        return allItems
    }
    private var defaultRule: DraftItem? { allItems.first(where: isDefaultRule) }
    private var enabledVisibleNodeIDs: [String] { items.filter(\.enabled).map(\.identifier) }

    private var nodeGroups: [NodeCollectionGroup] {
        guard descriptor.key == "nodes" else { return [] }
        var groups = [NodeCollectionGroup(
            id: "_manual", label: "手动节点",
            count: allItems.filter { $0.sourceSubscription == nil }.count
        )]
        var known = Set(["_manual"])
        for subscription in model.draftItems(for: "subscriptions") {
            known.insert(subscription.identifier)
            groups.append(NodeCollectionGroup(
                id: subscription.identifier, label: subscription.title,
                count: allItems.filter { $0.sourceSubscription == subscription.identifier }.count
            ))
        }
        for source in allItems.compactMap(\.sourceSubscription) where !known.contains(source) {
            known.insert(source)
            groups.append(NodeCollectionGroup(
                id: source, label: "缺失订阅：\(source)",
                count: allItems.filter { $0.sourceSubscription == source }.count
            ))
        }
        return groups
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label(descriptor.title, systemImage: descriptor.symbol)
                    .font(.headline)
                Text(descriptor.key == "nodes" ? "\(items.count) / \(allItems.count) 项" : "\(allItems.count) 项")
                    .foregroundStyle(.secondary)
                if descriptor.key == "nodes" {
                    Picker("节点分组", selection: Binding(
                        get: { activeNodeGroup },
                        set: { selectedNodeGroup = $0 }
                    )) {
                        ForEach(nodeGroups) { group in
                            Text("\(group.label) · \(group.count)").tag(group.id)
                        }
                    }
                    .pickerStyle(.menu)
                    .frame(maxWidth: 220)
                    .onChange(of: selectedNodeGroup) { _ in selection = nil }
                }
                Spacer()
                Button { editSelected() } label: {
                    Label("编辑", systemImage: "square.and.pencil")
                }
                .disabled(model.isBusy || selectedItem == nil || selectedItem?.subscriptionOwned == true)
                if descriptor.key == "nodes" {
                    Button {
                        selectedNodeGroup = "_manual"
                        nodeImportPresented = true
                    } label: {
                        Label("导入", systemImage: "square.and.arrow.down")
                    }
                    .disabled(model.isBusy)
                    Menu("批量测速") {
                        Button("批量连接测试") {
                            model.runAllNodeProbes(download: false, nodeIDs: enabledVisibleNodeIDs)
                        }
                        Button("批量下载测试") {
                            model.runAllNodeProbes(download: true, nodeIDs: enabledVisibleNodeIDs)
                        }
                    }
                    .disabled(model.isBusy || model.isDirty || model.isBatchNodeProbeRunning || enabledVisibleNodeIDs.isEmpty)
                }
                if descriptor.key == "subscriptions", let selectedItem {
                    Button("立即更新") { model.updateSubscription(selectedItem.identifier) }
                        .disabled(model.isBusy || model.isDirty
                                  || model.subscriptionOperationInProgress(selectedItem.identifier)
                                  || model.subscriptionStatus(selectedItem.identifier)?.enabled == false)
                        .help(model.subscriptionStatus(selectedItem.identifier)?.enabled == false
                              ? "已停用的订阅不能更新；请先启用并保存"
                              : "更新 Saved 节点库，不自动 Apply")
                }
                Button { addItem() } label: {
                    Label(descriptor.addLabel, systemImage: "plus")
                }
                .buttonStyle(.borderedProminent)
                .disabled(model.isBusy)
            }

            Table(of: DraftItem.self, selection: $selection) {
                TableColumn("状态") { item in
                    if item.subscriptionOwned || isRequiredDirect(item) {
                        Label(item.enabled ? "启用" : "停用", systemImage: "lock.fill")
                            .labelStyle(.iconOnly)
                            .foregroundStyle(item.enabled ? .green : .secondary)
                            .help(item.subscriptionOwned ? "订阅节点状态由订阅管理" : "Direct 是系统必需路由，始终启用")
                    } else {
                        Toggle("", isOn: Binding(
                            get: { item.enabled },
                            set: { model.setDraftItemEnabled(in: descriptor.key, at: item.index, enabled: $0) }
                        ))
                        .labelsHidden()
                        .toggleStyle(.switch)
                        .controlSize(.small)
                    }
                }
                .width(58)
                TableColumn("名称") { item in
                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            HStack(spacing: 6) {
                                Text(item.title).fontWeight(.medium)
                                if descriptor.key == "nodes", item.pinnedStale {
                                    Label("stale", systemImage: "pin.fill")
                                        .font(.caption)
                                        .foregroundStyle(.orange)
                                        .help("pinned-stale · 所属订阅 \(item.sourceSubscription ?? "未知")")
                                }
                            }
                        }
                        Spacer(minLength: 0)
                    }
                }
                TableColumn("类型") { item in
                    Text(kindLabel(item))
                        .font(.caption.weight(.medium))
                }
                .width(min: 80, ideal: 110)
                TableColumn("操作") { item in
                    HStack(spacing: 8) {
                        if descriptor.key == "nodes" {
                            probeButton(item: item, scope: "nodes", download: false)
                            probeButton(item: item, scope: "nodes", download: true)
                        } else if descriptor.key == "routes", item.kind == "single" {
                            probeButton(item: item, scope: "routes", download: false)
                            probeButton(item: item, scope: "routes", download: true)
                        } else if descriptor.key == "subscriptions" {
                            Button { model.updateSubscription(item.identifier) } label: {
                                if model.subscriptionOperationInProgress(item.identifier) {
                                    ProgressView().controlSize(.small)
                                } else {
                                    Image(systemName: "arrow.clockwise")
                                }
                            }
                            .buttonStyle(.borderless)
                            .disabled(model.isDirty || model.subscriptionOperationInProgress(item.identifier)
                                      || model.subscriptionStatus(item.identifier)?.enabled == false)
                            .help(model.subscriptionStatus(item.identifier)?.enabled == false
                                  ? "已停用的订阅不能更新"
                                  : "更新 Saved 节点库，不自动 Apply")
                        }
                        if item.subscriptionOwned {
                            Image(systemName: "lock.fill")
                                .foregroundStyle(.secondary)
                                .help("订阅节点只读")
                        } else {
                            Button { edit(item) } label: {
                                Image(systemName: "square.and.pencil")
                            }
                            .buttonStyle(.borderless)
                            if !isSystemRoute(item) && !isDefaultRule(item) {
                                Button(role: .destructive) {
                                    requestDeletion(item)
                                } label: {
                                    Image(systemName: "trash")
                                }
                                .buttonStyle(.borderless)
                            }
                        }
                    }
                }
                .width(min: 190, ideal: 230)
                TableColumn("详情") { item in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(runtimeDetail(item))
                            .font(.callout.monospaced())
                            .lineLimit(1)
                            .foregroundStyle(.secondary)
                        if let result = probeSummary(item) {
                            Text(result)
                                .font(.caption.monospaced())
                                .foregroundStyle(result.hasPrefix("失败") ? .red : .green)
                        }
                    }
                }
                TableColumn(descriptor.ordered ? "顺序" : "") { item in
                    if descriptor.ordered {
                        HStack(spacing: 7) {
                            Text(item.index + 1, format: .number)
                                .font(.caption.monospacedDigit())
                                .foregroundStyle(.secondary)
                            Image(systemName: isPinned(item) ? "pin.fill" : "line.3.horizontal")
                                .foregroundStyle(.secondary)
                        }
                        .frame(maxWidth: .infinity)
                        .help(isPinned(item) ? "Default 固定在最后" : "拖动调整顺序")
                        .accessibilityLabel(isPinned(item) ? "\(item.title) 固定在最后" : "拖动 \(item.title) 调整顺序")
                    }
                }
                .width(descriptor.ordered ? 78 : 1)
            } rows: {
                ForEach(items) { item in
                    TableRow(item)
                        .itemProvider {
                            descriptor.ordered && !isPinned(item) ? NSItemProvider(object: item.id as NSString) : nil
                        }
                }
                .dropDestination(for: String.self) { destination, identifiers in
                    if descriptor.ordered { moveItems(identifiers, to: destination) }
                }
            }
            .disabled(!model.canEditDraft)
            .overlay {
                if items.isEmpty {
                    VStack(spacing: 9) {
                        Image(systemName: descriptor.symbol)
                            .font(.largeTitle)
                            .foregroundStyle(.tertiary)
                        Text(descriptor.emptyMessage)
                            .font(.headline)
                        Text("使用右上角的“\(descriptor.addLabel)”创建工作副本条目。")
                            .font(.callout)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            .contextMenu(forSelectionType: DraftItem.ID.self) { selected in
                if let id = selected.first, let item = items.first(where: { $0.id == id }) {
                    if descriptor.ordered {
                        Button("上移") { model.moveDraftItem(in: descriptor.key, at: item.index, offset: -1) }
                            .disabled(item.index == 0 || isPinned(item))
                        Button("下移") { model.moveDraftItem(in: descriptor.key, at: item.index, offset: 1) }
                            .disabled(!canMoveDown(item))
                        Divider()
                    }
                    Button("编辑") { edit(item) }
                        .disabled(item.subscriptionOwned)
                    if !isSystemRoute(item) && !isDefaultRule(item) {
                        Button("删除", role: .destructive) {
                            requestDeletion(item)
                        }
                        .disabled(item.subscriptionOwned)
                    }
                }
            } primaryAction: { selected in
                if let id = selected.first, let item = items.first(where: { $0.id == id }) {
                    edit(item)
                }
            }

            if descriptor.key == "subscriptions", let selectedItem,
               let status = model.subscriptionStatus(selectedItem.identifier), !status.stale.isEmpty {
                SubscriptionStaleList(model: model, status: status)
            }

            if let defaultRule {
                DefaultRuleCard(
                    item: defaultRule,
                    detail: runtimeDetail(defaultRule),
                    isBusy: model.isBusy
                ) {
                    edit(defaultRule)
                }
            }

            HStack {
                if !actionError.isEmpty {
                    Label(actionError, systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(.red)
                } else if descriptor.key == "rules" {
                    Label("从上到下严格匹配；Default 必须位于最后。", systemImage: "info.circle")
                        .foregroundStyle(.secondary)
                } else if descriptor.key == "nodes" {
                    Label("订阅节点只读；手动节点可编辑。", systemImage: "info.circle")
                        .foregroundStyle(.secondary)
                }
                Spacer()
                if model.isDirty {
                    Label("工作副本已修改", systemImage: "circle.fill")
                        .foregroundStyle(.orange)
                }
            }
            .font(.caption)
        }
        .padding(24)
        .sheet(item: $editorTarget) { target in
            DraftItemEditor(model: model, target: target)
        }
        .sheet(isPresented: $nodeImportPresented) {
            NodeImportSheet(model: model)
        }
        .alert(
            "删除“\(pendingDeletion?.title ?? "项目")”？",
            isPresented: Binding(
                get: { pendingDeletion != nil },
                set: { if !$0 { pendingDeletion = nil } }
            )
        ) {
            Button("取消", role: .cancel) { pendingDeletion = nil }
            Button("删除", role: .destructive) {
                if let item = pendingDeletion {
                    model.removeDraftItem(from: descriptor.key, at: item.index)
                    selection = nil
                }
                pendingDeletion = nil
            }
        } message: {
            Text(descriptor.key == "subscriptions"
                 ? "相关订阅节点也会从工作副本移除。"
                 : "此操作只修改工作副本，应用前仍可重新载入系统配置。")
        }
    }

    private var selectedItem: DraftItem? {
        guard let selection else { return nil }
        return items.first { $0.id == selection }
    }

    private func addItem() {
        if descriptor.key == "nodes" { selectedNodeGroup = "_manual" }
        guard let object = model.newDraftItemObject(for: descriptor.key) else { return }
        editorTarget = DraftEditorTarget(
            key: descriptor.key, index: nil, title: descriptor.addLabel,
            object: object
        )
    }

    private func editSelected() {
        guard let selectedItem else { return }
        edit(selectedItem)
    }

    private func edit(_ item: DraftItem) {
        guard !item.subscriptionOwned,
              let object = model.draftItemObject(for: descriptor.key, at: item.index) else { return }
        editorTarget = DraftEditorTarget(
            key: descriptor.key, index: item.index, title: item.title,
            object: object
        )
    }

    private func requestDeletion(_ item: DraftItem) {
        if let reason = model.deletionBlockReason(for: descriptor.key, at: item.index) {
            actionError = reason
            return
        }
        actionError = ""
        pendingDeletion = item
    }

    private func moveItems(_ identifiers: [String], to destination: Int) {
        let sources = IndexSet(identifiers.compactMap { identifier in
            items.first(where: { $0.id == identifier })?.index
        })
        guard !sources.isEmpty, !sources.contains(where: { index in
            allItems.indices.contains(index) && isPinned(allItems[index])
        }) else { return }
        let resolvedDestination: Int
        if descriptor.key == "rules", let defaultIndex = allItems.firstIndex(where: isPinned) {
            resolvedDestination = min(destination, defaultIndex)
        } else {
            resolvedDestination = destination
        }
        model.moveDraftItem(in: descriptor.key, from: sources, to: resolvedDestination)
    }

    private func isPinned(_ item: DraftItem) -> Bool {
        descriptor.key == "rules" && item.kind.caseInsensitiveCompare("default") == .orderedSame
    }

    private func isRequiredDirect(_ item: DraftItem) -> Bool {
        descriptor.key == "routes" && item.kind.caseInsensitiveCompare("direct") == .orderedSame
    }

    private func kindLabel(_ item: DraftItem) -> String {
        if descriptor.key == "routes", item.kind.caseInsensitiveCompare("block") == .orderedSame {
            return "REJECT"
        }
        return item.kind.isEmpty ? "—" : item.kind.uppercased()
    }

    private func isSystemRoute(_ item: DraftItem) -> Bool {
        descriptor.key == "routes" && ["direct", "block"].contains(item.kind.lowercased())
    }

    private func isDefaultRule(_ item: DraftItem) -> Bool {
        descriptor.key == "rules" && item.kind.caseInsensitiveCompare("default") == .orderedSame
    }

    private func canMoveDown(_ item: DraftItem) -> Bool {
        guard !isPinned(item),
              let position = items.firstIndex(where: { $0.id == item.id }),
              position < items.count - 1 else { return false }
        return !isPinned(items[position + 1])
    }

    private func probeSummary(_ item: DraftItem) -> String? {
        guard descriptor.key == "nodes" || descriptor.key == "routes" else { return nil }
        let prefix = "\(descriptor.key):\(item.identifier):"
        let connect = model.probeSummaries[prefix + "connect"]
        let download = model.probeSummaries[prefix + "download"]
        return [connect.map { "连接 \($0)" }, download.map { "下载 \($0)" }].compactMap { $0 }.joined(separator: " · ").nilIfEmpty
    }

    @ViewBuilder
    private func probeButton(item: DraftItem, scope: String, download: Bool) -> some View {
        let running = model.probeInProgress(scope: scope, objectID: item.identifier, download: download)
        Button {
            model.runProbe(
                kind: "speedtest",
                nodeID: scope == "nodes" ? item.identifier : nil,
                routeID: scope == "routes" ? item.identifier : nil,
                download: download
            )
        } label: {
            if running {
                ProgressView().controlSize(.small)
            } else {
                Label(download ? "下载" : "连接", systemImage: download ? "arrow.down.circle" : "network")
                    .font(.caption)
            }
        }
        .buttonStyle(.borderless)
        .help(!item.enabled ? "已停用对象不能测试" : (download ? "下载测速" : (scope == "routes" ? "路由链连接测试" : "连接测试")))
        .disabled(model.isDirty || running || !item.enabled)
    }

    private func runtimeDetail(_ item: DraftItem) -> String {
        guard descriptor.key == "subscriptions", let status = model.subscriptionStatus(item.identifier) else {
            return item.detail.isEmpty ? "—" : item.detail
        }
        let success = status.lastSuccess ?? "—"
        let failure = status.lastFailure.map { " · 最近失败 \($0.summary)" } ?? ""
        return "\(status.stateLabel) · \(status.inventorySummary) · 最近成功 \(success)\(failure)"
    }
}

private extension String {
    var nilIfEmpty: String? { isEmpty ? nil : self }
}

struct DiagnosticsView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        List {
            Section("连通性探测") {
                Text("目标来自当前 Active generation，并按 Active 规则访问。成功只表示该 URL 当时可达，不证明具体 outbound、DNS resolver 或 DNS 无泄漏。")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                HStack {
                    overviewProbeButton("测试直连 URL", kind: "direct")
                    overviewProbeButton("测试代理 URL", kind: "proxy")
                    overviewProbeButton("测试下载 URL", kind: "speedtest", download: true)
                }
                if !model.hasActiveGeneration {
                    Label("当前没有 Active generation；概览探测不会发起请求。", systemImage: "pause.circle")
                        .foregroundStyle(.secondary)
                }
                overviewProbeResult("直连 URL", kind: "direct")
                overviewProbeResult("代理 URL", kind: "proxy")
                overviewProbeResult("下载 URL", kind: "speedtest")
            }
            Section("最近 Probe 报告") {
                if model.diagnosticProbeReports.isEmpty {
                    Label("尚无已保存报告", systemImage: "doc.text.magnifyingglass")
                        .foregroundStyle(.secondary)
                }
                ForEach(model.diagnosticProbeReports) { report in
                    probeReport(report)
                }
                ForEach(model.diagnosticsWarnings, id: \.self) { warning in
                    Label(warning, systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.orange)
                }
            }
            Section("配置校验") {
                if let validation = model.validation {
                    LabeledContent("结果", value: validation.ok ? "通过" : "失败")
                    ForEach(validation.errors) { issue in
                        issueRow(issue, isError: true)
                    }
                    ForEach(validation.warnings) { issue in
                        issueRow(issue, isError: false)
                    }
                } else {
                    Label("尚未校验", systemImage: "questionmark.circle")
                        .foregroundStyle(.secondary)
                }
                Button("校验当前工作副本") { model.validate() }
                    .disabled(model.isBusy)
            }
            Section("运行后端") {
                LabeledContent("Service", value: "LaunchDaemon + sing-box TUN")
                LabeledContent("健康状态", value: model.runtime.healthy ? "正常" : "未运行")
                LabeledContent("Generation", value: model.runtime.generationID.isEmpty ? "—" : model.runtime.generationID)
                if !model.runtime.error.isEmpty {
                    LabeledContent("错误", value: model.runtime.error)
                }
                Button("刷新状态") { model.refreshStatus() }
                    .disabled(model.isBusy)
            }
            Section("最近 Apply") {
                if let apply = model.runtime.lastApply {
                    LabeledContent("Sequence", value: apply.sequence)
                    LabeledContent("结果", value: apply.result.ok ? "成功" : "失败")
                    if let generation = apply.result.generation, !generation.isEmpty {
                        LabeledContent("Generation", value: generation)
                    }
                    if let error = apply.result.error, !error.isEmpty {
                        LabeledContent("错误", value: error)
                    }
                } else {
                    Text("尚无 Apply 记录").foregroundStyle(.secondary)
                }
            }
            Section("最近消息") {
                Text(model.message.isEmpty ? "—" : model.message)
                    .textSelection(.enabled)
            }
            Section("最近日志") {
                if model.diagnosticsLog.isEmpty {
                    Label("尚未读取日志", systemImage: "doc.text.magnifyingglass")
                        .foregroundStyle(.secondary)
                } else {
                    ScrollView([.horizontal, .vertical]) {
                        Text(model.diagnosticsLog)
                            .font(.system(.caption, design: .monospaced))
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .topLeading)
                    }
                    .frame(minHeight: 220, maxHeight: 320)
                }
                Button("刷新日志") { model.refreshLogs() }
                    .disabled(model.isBusy)
            }
            Section {
                Button("刷新全部诊断") { model.refreshDiagnostics() }
                    .disabled(model.isBusy)
            }
        }
        .listStyle(.inset)
        .task {
            if model.diagnosticsLog.isEmpty { model.refreshLogs() }
        }
    }

    @ViewBuilder
    private func overviewProbeButton(_ title: String, kind: String, download: Bool = false) -> some View {
        let running = model.overviewProbeInProgress(kind)
        Button {
            model.runProbe(kind: kind, download: download)
        } label: {
            if running {
                HStack(spacing: 6) {
                    ProgressView().controlSize(.small)
                    Text(title)
                }
            } else {
                Text(title)
            }
        }
        .disabled(running || !model.hasActiveGeneration)
        .help(model.hasActiveGeneration ? "目标来自 Active generation，并按 Active 规则访问" : "需要 Active generation")
    }

    @ViewBuilder
    private func overviewProbeResult(_ title: String, kind: String) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            LabeledContent(title, value: model.overviewProbeSummary(kind))
            if let detail = model.overviewProbeDetail(kind) {
                Text(detail)
                    .font(.caption2.monospaced())
                    .foregroundStyle(model.overviewProbeIsStale(kind) ? Color.orange : Color.secondary)
                    .textSelection(.enabled)
            }
        }
    }

    @ViewBuilder
    private func probeReport(_ report: ProbeReport) -> some View {
        let stale = model.probeReportIsStale(report)
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(report.scope == "overview" ? "Overview" : "\(report.scope)/\(report.objectID ?? "—")")
                    .fontWeight(.medium)
                Text(report.kind).font(.caption.monospaced()).foregroundStyle(.secondary)
                Spacer()
                Label(stale ? "已过期" : (report.ok ? "成功" : "失败"),
                      systemImage: stale ? "clock.badge.exclamationmark" : (report.ok ? "checkmark.circle.fill" : "xmark.circle.fill"))
                    .font(.caption)
                    .foregroundStyle(stale ? .orange : (report.ok ? .green : .red))
            }
            LabeledContent("tested_at", value: report.testedAt)
            if let result = report.results.first {
                if let url = result.url, !url.isEmpty { LabeledContent("URL", value: url) }
                if let attempts = result.attempts { LabeledContent("Attempts", value: String(attempts)) }
                HStack(spacing: 14) {
                    measurement("Connect", result.connectMilliseconds, "ms")
                    measurement("TLS", result.tlsMilliseconds, "ms")
                    measurement("TTFB", result.firstByteMilliseconds, "ms")
                    measurement("HTTP", result.status, "")
                }
                if let bytes = result.downloadedBytes, bytes > 0 {
                    let milliseconds = result.downloadMilliseconds ?? 0
                    let rate = milliseconds > 0 ? String(format: "%.1f Mbps", Double(bytes) * 8 / Double(milliseconds) / 1000) : "—"
                    Text("\(bytes) bytes · \(milliseconds) ms · \(rate)")
                        .font(.caption.monospaced())
                }
                if let error = result.error, !error.isEmpty { Text(error).foregroundStyle(.red) }
            }
            if let error = report.error, !error.isEmpty { Text(error).foregroundStyle(.red) }
        }
        .padding(.vertical, 4)
    }

    @ViewBuilder
    private func measurement(_ label: String, _ value: Int?, _ suffix: String) -> some View {
        if let value {
            Text("\(label) \(value)\(suffix)").font(.caption.monospaced())
        }
    }

    private func issueRow(_ issue: ValidationIssue, isError: Bool) -> some View {
        Button {
            if let page = issue.destinationPage { model.selectedPage = page }
        } label: {
            HStack {
                Label(issue.message, systemImage: isError ? "xmark.octagon.fill" : "exclamationmark.triangle.fill")
                    .foregroundStyle(isError ? .red : .orange)
                Spacer()
                if issue.destinationPage != nil {
                    Image(systemName: "chevron.right")
                        .foregroundStyle(.tertiary)
                }
            }
        }
        .buttonStyle(.plain)
        .disabled(issue.destinationPage == nil)
    }
}

struct SystemView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        List {
            Section("系统组件") {
                LabeledContent("安装状态") {
                    Label(
                        model.systemComponentsUpdateAvailable ? "可更新" : (model.systemComponentsInstalled ? "已安装" : "未安装"),
                        systemImage: model.systemComponentsUpdateAvailable
                            ? "arrow.down.circle.fill"
                            : (model.systemComponentsInstalled ? "checkmark.seal.fill" : "shippingbox")
                    )
                    .foregroundStyle(model.systemComponentsInstalled && !model.systemComponentsUpdateAvailable ? .green : .orange)
                }
                if model.systemComponentsUpdateAvailable, model.embeddedInstallerAvailable {
                    Button("更新系统组件…") { model.installSystemComponents() }
                        .buttonStyle(.borderedProminent)
                        .disabled(model.isBusy)
                    Text("更新会再次请求一次管理员密码；用户配置和运行状态目录会保留。")
                        .foregroundStyle(.secondary)
                } else if !model.systemComponentsInstalled {
                    if model.embeddedInstallerAvailable {
                        Button("安装系统组件…") { model.installSystemComponents() }
                            .buttonStyle(.borderedProminent)
                            .disabled(model.isBusy)
                        Text("首次安装会请求一次管理员密码，并安装 root control daemon、运行 helper、sing-box 与 Geo seed。")
                            .foregroundStyle(.secondary)
                    } else {
                        Text("当前源码开发构建没有内置 payload，请运行 macos/scripts/install-launchdaemon.sh。")
                            .foregroundStyle(.secondary)
                    }
                }
            }
            Section("版本与运行时") {
                LabeledContent("Canonical schema", value: model.draftSchemaVersion == 0 ? "—" : String(model.draftSchemaVersion))
                LabeledContent("Backend", value: "steer-macos \(model.versions.helper)")
                LabeledContent("数据面", value: "sing-box \(model.versions.singBox) · TUN")
                LabeledContent("LaunchDaemon", value: "com.steer.steer")
                LabeledContent("GeoSite selectors", value: model.geositeNames.count.formatted())
                LabeledContent("GeoIP categories", value: model.geoipNames.count.formatted())
            }
            Section("存储路径") {
                pathRow("配置", "/Library/Application Support/Steer/config/config.json")
                pathRow("运行目录", "/Library/Application Support/Steer/run")
                pathRow("状态目录", "/Library/Application Support/Steer/state")
                pathRow("Geo Seed", "/Library/Application Support/Steer/geodata-seed")
                pathRow("日志", "/Library/Logs/Steer")
                pathRow("控制 IPC", "/var/run/steer/control.sock")
            }
            Section("授权") {
                Text("首次安装系统组件需要一次 macOS 管理员授权。之后保存和应用只通过 root control daemon 的受限 Unix socket IPC；服务会同时校验 socket 权限与调用者的 admin 组凭据，不执行任意命令。")
                    .foregroundStyle(.secondary)
            }
        }
        .listStyle(.inset)
    }

    private func pathRow(_ title: String, _ path: String) -> some View {
        LabeledContent(title) {
            Text(path)
                .font(.callout.monospaced())
                .textSelection(.enabled)
        }
    }
}

private extension AppPage {
    init?(contractKey: String) {
        switch contractKey {
        case "overview": self = .overview
        case "general": self = .general
        case "nodes": self = .nodes
        case "routes": self = .routes
        case "dns": self = .dns
        case "proxies": self = .proxies
        case "rules": self = .rules
        case "subscriptions": self = .subscriptions
        case "diagnostics": self = .diagnostics
        case "system": self = .settings
        case "advanced": self = .configuration
        default: return nil
        }
    }

    var navigationLabel: String {
        switch self {
        case .overview: return "总览"
        case .general: return "基础设置"
        case .configuration: return "Canonical JSON · 高级"
        case .nodes: return "节点"
        case .routes: return "路由"
        case .dns: return "DNS Profile"
        case .rules: return "规则"
        case .subscriptions: return "订阅"
        case .proxies: return "本地代理"
        case .diagnostics: return "诊断"
        case .settings: return "系统"
        }
    }

    var eyebrow: String {
        switch self {
        case .overview: return "Steer for macOS"
        case .general: return "Configuration"
        case .configuration: return "Canonical JSON"
        case .nodes: return "Nodes"
        case .routes: return "Routes"
        case .dns: return "DNS Profiles"
        case .rules: return "First-Match"
        case .subscriptions: return "Subscriptions"
        case .proxies: return "Local Proxies"
        case .diagnostics: return "Diagnostics"
        case .settings: return "System"
        }
    }

    var subtitle: String {
        switch self {
        case .overview: return "透明代理控制面 · Canonical Intent 摘要与运行状态"
        case .general: return "运行、探测、DNS 缓存与 Bootstrap 的原生字段设置"
        case .configuration: return "高级编辑与校验；所有可视化页面共享同一份工作副本"
        case .nodes: return "手动节点可编辑，订阅节点保持只读"
        case .routes: return "Direct、Reject 与 Single Route 的确定性出口关系"
        case .dns: return "上游解析器与每条规则的独立 DNS 路径"
        case .rules: return "从上到下严格匹配，Default 必须位于最后"
        case .subscriptions: return "订阅源、节点同步与 stale 清理"
        case .proxies: return "本机 SOCKS、HTTP 与 Mixed 入口"
        case .diagnostics: return "完整 Probe 报告、配置校验、最近 Apply 与相关日志"
        case .settings: return "版本、运行时与系统路径"
        }
    }
}

private extension ValidationIssue {
    var destinationPage: AppPage? {
        switch objectType {
        case "steer", "bootstrap": return .general
        case "node": return .nodes
        case "route": return .routes
        case "dns_profile": return .dns
        case "local_proxy": return .proxies
        case "rule": return .rules
        case "subscription": return .subscriptions
        default: return nil
        }
    }
}
