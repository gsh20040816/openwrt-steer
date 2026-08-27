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
            Label("应用已保存配置", systemImage: "bolt")
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
                Label(
                    model.runtime.healthy ? "运行正常" : (model.runtime.generationID.isEmpty ? "未运行" : "运行异常"),
                    systemImage: model.runtime.healthy ? "checkmark.circle.fill" : (model.runtime.generationID.isEmpty ? "circle" : "exclamationmark.triangle.fill")
                )
                .foregroundStyle(model.runtime.healthy ? .green : (model.runtime.generationID.isEmpty ? .secondary : .orange))
                Button { model.refreshStatus() } label: {
                    Label("刷新状态", systemImage: "arrow.clockwise")
                }
                .help("刷新服务运行状态")
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
            "配置冲突",
            isPresented: Binding(
                get: { model.revisionConflict != nil },
                set: { _ in }
            )
        ) {
            Button("重新载入服务器配置", role: .destructive) {
                model.reloadSavedAfterRevisionConflict()
            }
            Button("保留本地工作副本", role: .cancel) {
                model.keepLocalDraftAfterRevisionConflict()
            }
            Button("覆盖服务器配置", role: .destructive) {
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
                Image(systemName: model.runtime.healthy ? "checkmark.circle.fill" : (model.runtime.generationID.isEmpty ? "circle" : "exclamationmark.triangle.fill"))
                    .foregroundStyle(model.runtime.healthy ? .green : (model.runtime.generationID.isEmpty ? .secondary : .orange))
                VStack(alignment: .leading, spacing: 1) {
                    Text(model.runtime.healthy ? "服务运行正常" : (model.runtime.generationID.isEmpty ? "服务未运行" : "服务运行异常"))
                        .font(.caption.weight(.medium))
                    Text(model.isDirty ? "有未保存修改" : "配置已保存")
                        .font(.caption2)
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
                    Label(
                        model.runtime.healthy ? "服务运行正常" : (model.runtime.generationID.isEmpty ? "服务未运行" : "服务运行异常"),
                        systemImage: model.runtime.healthy ? "checkmark.circle.fill" : (model.runtime.generationID.isEmpty ? "pause.circle" : "exclamationmark.triangle.fill")
                    )
                        .font(.title.weight(.semibold))
                        .foregroundStyle(model.runtime.healthy ? .green : (model.runtime.generationID.isEmpty ? .secondary : .orange))
                    Text("系统 TUN 模式")
                        .foregroundStyle(.secondary)
                }
                Spacer(minLength: 20)
                VStack(alignment: .trailing, spacing: 12) {
                    Toggle("启用配置", isOn: Binding(
                        get: { model.draftEnabled },
                        set: { model.setEnabledAndApply($0) }
                    ))
                    .toggleStyle(.switch)
                    .disabled(!model.canToggleEnabled)
                    .help(model.isDirty ? "请先保存或丢弃工作副本中的修改" : "立即保存并应用启用状态")
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
            pipelineStep(value: model.itemCount(for: "rules"), title: "匹配规则", subtitle: "首条匹配即停止", symbol: "line.3.horizontal.decrease.circle")
            pipelineArrow
            pipelineStep(value: model.itemCount(for: "dns_profiles"), title: "DNS Profile", subtitle: "独立解析路径", symbol: "network")
            pipelineArrow
            pipelineStep(value: model.itemCount(for: "routes"), title: "路由", subtitle: "直连 / 拒绝 / 节点链", symbol: "arrow.triangle.branch")
            pipelineArrow
            pipelineStep(value: 1, title: "网络出口", subtitle: "本地出口", symbol: "globe")
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
            GridRow {
                metric(value: model.itemCount(for: "local_proxies"), label: "本地入口", footnote: "\(model.enabledItemCount(for: "local_proxies")) 已启用", symbol: "rectangle.connected.to.line.below")
                metric(value: model.itemCount(for: "subscriptions"), label: "订阅", footnote: "\(model.enabledItemCount(for: "subscriptions")) 已启用", symbol: "arrow.down.circle")
                metric(value: model.validation?.warnings.count ?? 0, label: "警告", footnote: model.validation == nil ? "尚未校验" : "当前工作副本", symbol: "exclamationmark.triangle")
                metric(value: model.validation?.errors.count ?? 0, label: "错误", footnote: model.validation == nil ? "尚未校验" : "当前工作副本", symbol: "xmark.octagon")
            }
        }
    }

    private var configurationContent: some View {
        Grid(alignment: .leading, horizontalSpacing: 28, verticalSpacing: 10) {
            GridRow {
                LabeledContent("日志级别", value: model.draftLogLevel)
                LabeledContent("DNS 缓存", value: model.draftDNSCacheCapacityLabel)
            }
            GridRow {
                LabeledContent("工作副本", value: model.isDirty ? "有未保存修改" : "已同步")
                LabeledContent("配置开关", value: model.draftEnabled ? "启用" : "禁用")
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
                Label("JSON 配置", systemImage: "doc.plaintext")
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
    static let rules = Self(key: "rules", title: "顺序规则", emptyMessage: "尚未添加规则", addLabel: "添加规则", symbol: "list.number", ordered: true)
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
                Label("已失效节点", systemImage: "clock.badge.exclamationmark")
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
                            Text(node.name?.isEmpty == false ? node.name! : "未命名节点")
                                .fontWeight(.medium)
                            Label("已保留", systemImage: "pin.fill")
                                .font(.caption)
                                .foregroundStyle(.orange)
                        }
                        Text(node.referencedBy.isEmpty
                             ? "未被任何路由使用，可以安全清理"
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
                    .help(node.referencedBy.isEmpty ? "只移除这个已失效节点；不会改变当前运行配置" : "仍被路由使用，不能清理")
                }
                .padding(.vertical, 3)
            }
        }
        .padding(14)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color(nsColor: .controlBackgroundColor)))
        .overlay(RoundedRectangle(cornerRadius: 10).stroke(.quaternary))
    }

    private func referenceLabel(_ reference: SubscriptionReference) -> String {
        let type = ["route": "路由", "rule": "规则"][reference.objectType] ?? "配置"
        let label = reference.name?.isEmpty == false ? reference.name! : "未命名\(type)"
        return "\(type) \(label)"
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
            Text("第 \(item.index + 1) 条 · 只能修改显示名称、DNS Profile 与路由")
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
    @State private var blockedReferences: [UIObjectReference] = []
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
                id: source, label: "已删除的订阅",
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
                .disabled(model.isBusy || model.draftSyntaxError != nil
                          || selectedItem == nil || selectedItem?.subscriptionOwned == true)
                if descriptor.key == "nodes" {
                    Button {
                        selectedNodeGroup = "_manual"
                        nodeImportPresented = true
                    } label: {
                        Label("导入", systemImage: "square.and.arrow.down")
                    }
                    .disabled(model.isBusy || model.draftSyntaxError != nil)
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
                              : "更新已保存的节点列表，不改变当前运行配置")
                }
                Button { addItem() } label: {
                    Label(descriptor.addLabel, systemImage: "plus")
                }
                .buttonStyle(.borderedProminent)
                .disabled(model.isBusy || model.draftSyntaxError != nil)
            }

            if descriptor.key == "subscriptions" {
                Label("订阅会定时刷新节点列表；仍被路由使用的节点将自动保留。", systemImage: "info.circle")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            Table(of: DraftItem.self, selection: $selection) {
                TableColumn("状态") { item in
                    if item.subscriptionOwned || isRequiredDirect(item) {
                        Label(item.enabled ? "启用" : "停用", systemImage: "lock.fill")
                            .labelStyle(.iconOnly)
                            .foregroundStyle(item.enabled ? .green : .secondary)
                            .help(item.subscriptionOwned ? "订阅节点状态由订阅管理" : "系统直连路由始终启用")
                    } else {
                        Toggle("", isOn: Binding(
                            get: {
                                model.draftItemEnabled(
                                    in: descriptor.key,
                                    identifiedBy: item.identifier
                                ) ?? item.enabled
                            },
                            set: {
                                model.setDraftItemEnabled(
                                    in: descriptor.key,
                                    identifiedBy: item.identifier,
                                    enabled: $0
                                )
                            }
                        ))
                        .labelsHidden()
                        .toggleStyle(.switch)
                        .controlSize(.small)
                        .id("\(item.id):enabled")
                    }
                }
                .width(58)
                TableColumn("名称") { item in
                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            HStack(spacing: 6) {
                                Text(item.title).fontWeight(.medium)
                                if descriptor.key == "nodes", item.pinnedStale {
                                    Label("已失效", systemImage: "pin.fill")
                                        .font(.caption)
                                        .foregroundStyle(.orange)
                                        .help("原订阅中已不再包含此节点；因仍在使用而暂时保留")
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
                                  : "更新已保存的节点列表，不改变当前运行配置")
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
                            .lineLimit(descriptor.key == "rules" ? 3 : 1)
                            .foregroundStyle(.secondary)
                            .help(runtimeDetail(item))
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
            .disabled(!model.canEditDraft || model.draftSyntaxError != nil)
            .overlay {
                if let syntaxError = model.draftSyntaxError {
                    VStack(spacing: 9) {
                        Image(systemName: "xmark.octagon.fill")
                            .font(.largeTitle)
                            .foregroundStyle(.red)
                        Text("JSON 配置格式有误")
                            .font(.headline)
                        Text(syntaxError)
                            .font(.callout.monospaced())
                            .foregroundStyle(.secondary)
                            .multilineTextAlignment(.center)
                            .lineLimit(4)
                        Text("请先在高级配置页面修复后再继续。")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .padding(24)
                } else if items.isEmpty {
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
            if !blockedReferences.isEmpty {
                HStack(spacing: 8) {
                    Text("先修改引用：").foregroundStyle(.secondary)
                    ForEach(blockedReferences, id: \.sourceID) { reference in
                        Button(reference.sourceLabel) {
                            model.focusValidationIssue(ValidationIssue(
                                code: "STILL_REFERENCED", objectType: reference.sourceObjectType,
                                objectID: reference.sourceID, option: reference.field,
                                message: "对象仍引用待删除项目"
                            ))
                        }
                        .buttonStyle(.link)
                    }
                }
                .font(.caption)
            }
        }
        .padding(24)
        .sheet(item: $editorTarget) { target in
            DraftItemEditor(model: model, target: target)
        }
        .sheet(isPresented: $nodeImportPresented) {
            NodeImportSheet(model: model)
        }
        .onAppear { openValidationFocus() }
        .onChange(of: model.validationFocus?.id) { _ in openValidationFocus() }
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
            blockedReferences = model.deletionReferences(for: descriptor.key, at: item.index)
            return
        }
        actionError = ""
        blockedReferences = []
        pendingDeletion = item
    }

    private func openValidationFocus() {
        let objectType = [
            "nodes": "node", "routes": "route", "dns_profiles": "dns_profile",
            "local_proxies": "local_proxy", "rules": "rule", "subscriptions": "subscription",
        ][descriptor.key]
        guard let objectType,
              let issue = model.takeValidationFocus(objectType: objectType),
              let objectID = issue.objectID,
              let item = allItems.first(where: { $0.identifier == objectID }) else { return }
        if descriptor.key == "nodes" { selectedNodeGroup = item.sourceSubscription ?? "_manual" }
        selection = item.id
        guard !item.subscriptionOwned,
              let object = model.draftItemObject(for: descriptor.key, at: item.index) else {
            actionError = "已定位只读对象 \(item.title) / \(issue.option ?? "")"
            return
        }
        editorTarget = DraftEditorTarget(
            key: descriptor.key, index: item.index, title: item.title,
            object: object, focusOption: issue.option
        )
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
                Text("使用设备当前网络环境访问已保存的测试地址；即使 Steer 未启用也可以测试。成功仅表示该地址在测试时可达。")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                HStack {
                    overviewProbeButton("测试直连 URL", kind: "direct")
                    overviewProbeButton("测试代理 URL", kind: "proxy")
                    overviewProbeButton("测试下载 URL", kind: "speedtest", download: true)
                }
                overviewProbeResult("直连 URL", kind: "direct")
                overviewProbeResult("代理 URL", kind: "proxy")
                overviewProbeResult("下载 URL", kind: "speedtest")
            }
            Section("最近测试报告") {
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
            Section("系统 DNS 接管检查") {
                if let capture = model.diagnosticsDNSCapture {
                    LabeledContent("结果", value: capture.configured ? "已配置" : "未确认")
                    LabeledContent(
                        "详情",
                        value: capture.detail == "the published Active generation contains the expected port-53 capture artifacts"
                            ? "系统 DNS 接管已配置"
                            : capture.detail
                    )
                } else {
                    Label("尚未检查系统 DNS 接管状态", systemImage: "questionmark.circle")
                        .foregroundStyle(.secondary)
                }
            }
            Section("服务状态") {
                LabeledContent("运行状态", value: model.runtime.healthy ? "正常" : (model.runtime.generationID.isEmpty ? "未运行" : "异常"))
                if !model.runtime.error.isEmpty {
                    LabeledContent("错误", value: model.runtime.error)
                }
                Button("刷新状态") { model.refreshStatus() }
                    .disabled(model.isBusy)
            }
            Section("最近应用") {
                if let apply = model.runtime.lastApply {
                    LabeledContent("结果", value: apply.result.ok ? "成功" : "失败")
                    if let error = apply.result.error, !error.isEmpty {
                        LabeledContent("错误", value: error)
                    }
                } else {
                    Text("尚无应用记录").foregroundStyle(.secondary)
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
        .disabled(running)
        .help("使用设备当前网络环境进行测试")
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
                Text(reportScopeLabel(report.scope))
                    .fontWeight(.medium)
                Text(reportKindLabel(report.kind)).font(.caption).foregroundStyle(.secondary)
                Spacer()
                Label(stale ? "已过期" : (report.ok ? "成功" : "失败"),
                      systemImage: stale ? "clock.badge.exclamationmark" : (report.ok ? "checkmark.circle.fill" : "xmark.circle.fill"))
                    .font(.caption)
                    .foregroundStyle(stale ? .orange : (report.ok ? .green : .red))
            }
            LabeledContent("测试时间", value: report.testedAt)
            if let result = report.results.first {
                if let url = result.url, !url.isEmpty { LabeledContent("URL", value: url) }
                if let attempts = result.attempts { LabeledContent("尝试次数", value: String(attempts)) }
                HStack(spacing: 14) {
                    measurement("Connect", result.connectMilliseconds, "ms")
                    measurement("TLS", result.tlsMilliseconds, "ms")
                    measurement("TTFB", result.firstByteMilliseconds, "ms")
                    measurement("HTTP", result.status, "")
                }
                if let bytes = result.downloadedBytes, bytes > 0 {
                    let milliseconds = result.downloadMilliseconds ?? 0
                    let rate = milliseconds > 0 ? String(format: "%.1f Mbps", Double(bytes) * 8 / Double(milliseconds) / 1000) : "—"
                    Text("\(bytes) 字节 · \(milliseconds) ms · \(rate)")
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
        return Button {
            model.focusValidationIssue(issue)
        } label: {
            HStack {
                Label(validationIssueMessage(issue), systemImage: isError ? "xmark.octagon.fill" : "exclamationmark.triangle.fill")
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

    private func reportScopeLabel(_ scope: String) -> String {
        switch scope {
        case "overview": return "总览"
        case "node", "nodes": return "节点"
        case "route", "routes": return "路由"
        default: return "测试对象"
        }
    }

    private func validationIssueMessage(_ issue: ValidationIssue) -> String {
        switch issue.code {
        case "REQUIRED": return "必填字段尚未填写"
        case "DANGLING_NODE": return "所选节点不存在"
        case "DANGLING_DETOUR": return "所选前置路由不存在"
        case "DANGLING_DNS_PROFILE": return "所选 DNS Profile 不存在"
        case "DANGLING_ROUTE": return "所选路由不存在"
        case "DANGLING_LOCAL_PROXY": return "所选本地代理入口不存在"
        case "DISABLED_NODE": return "所选节点已停用"
        case "DISABLED_DETOUR": return "所选前置路由已停用"
        case "DISABLED_DNS_PROFILE": return "所选 DNS Profile 已停用"
        case "DISABLED_ROUTE": return "所选路由已停用"
        case "DISABLED_LOCAL_PROXY": return "所选本地代理入口已停用"
        case "ROUTE_DETOUR_CYCLE": return "前置代理链存在循环引用"
        case "LOCAL_PROXY_AUTH_REQUIRED": return "该监听地址允许其他设备连接，必须设置用户名和密码"
        case "INVALID_DURATION": return "更新间隔必须是大于零的时长"
        case "DNS_PROJECTION_EMPTY": return "该规则仅含连接阶段条件，不影响 DNS 上游选择"
        default: return issue.message
        }
    }

    private func reportKindLabel(_ kind: String) -> String {
        switch kind {
        case "direct": return "直连测试"
        case "proxy": return "代理测试"
        case "speedtest", "download": return "下载测试"
        case "connect": return "连接测试"
        default: return "连通性测试"
        }
    }
}

struct SystemView: View {
    @ObservedObject var model: AppModel
    @State private var showingUninstall = false
    @State private var showingDeleteUserData = false

    var body: some View {
        List {
            Section("系统组件") {
                LabeledContent("安装状态") {
                    Label(
                        model.systemComponentsUpdateAvailable ? "可更新" : (model.systemComponentsNeedRepair ? "安装不完整" : (model.systemComponentsInstalled ? "已安装" : "未安装")),
                        systemImage: model.systemComponentsUpdateAvailable
                            ? "arrow.down.circle.fill"
                            : (model.systemComponentsNeedRepair ? "wrench.and.screwdriver.fill" : (model.systemComponentsInstalled ? "checkmark.seal.fill" : "shippingbox"))
                    )
                    .foregroundStyle(model.systemComponentsInstalled && !model.systemComponentsUpdateAvailable ? .green : .orange)
                }
                if model.systemComponentsUpdateAvailable, model.embeddedInstallerAvailable {
                    Button("更新系统组件…") { model.installSystemComponents() }
                        .buttonStyle(.borderedProminent)
                        .disabled(model.isBusy)
                    Text("更新会再次请求管理员授权；用户配置和运行状态会保留。")
                        .foregroundStyle(.secondary)
                } else if model.systemComponentsNeedRepair, model.embeddedInstallerAvailable {
                    Button("修复系统组件…") { model.installSystemComponents() }
                        .buttonStyle(.borderedProminent)
                        .disabled(model.isBusy)
                    Text("修复会补齐缺失或损坏的组件，并保留用户配置、运行状态和当前工作副本。")
                        .foregroundStyle(.secondary)
                } else if !model.systemComponentsInstalled {
                    if model.embeddedInstallerAvailable {
                        Button("安装系统组件…") { model.installSystemComponents() }
                            .buttonStyle(.borderedProminent)
                            .disabled(model.isBusy)
                        Text("首次安装会请求管理员授权，并安装运行 Steer 所需的系统组件。")
                            .foregroundStyle(.secondary)
                    } else {
                        Text("当前开发构建不包含安装组件；请使用项目提供的安装脚本。")
                            .foregroundStyle(.secondary)
                    }
                }
                if model.systemComponentsHaveArtifacts {
                    if model.embeddedUninstallerAvailable {
                        Button("卸载系统组件…", role: .destructive) { showingUninstall = true }
                            .disabled(model.isBusy)
                    } else {
                        Text("当前 App 不包含受控卸载器，不能从 GUI 卸载程序组件。")
                            .foregroundStyle(.secondary)
                    }
                }
                Text("默认卸载会保留用户配置、运行状态和日志。")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if !model.systemComponentFacts.isEmpty {
                Section("组件详情") {
                    ForEach(model.systemComponentFacts) { fact in
                        VStack(alignment: .leading, spacing: 5) {
                            HStack {
                                Label(fact.label, systemImage: componentSymbol(fact.state))
                                    .foregroundStyle(componentColor(fact.state))
                                Spacer()
                                Text(fact.detail)
                                    .foregroundStyle(fact.ready ? .secondary : componentColor(fact.state))
                            }
                            Text(fact.path)
                                .font(.caption.monospaced())
                                .foregroundStyle(.secondary)
                                .textSelection(.enabled)
                            if !fact.requiredForInstallation, fact.state == .inactive {
                                Text("运行服务未激活不影响系统组件安装完整性。")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            Section("版本与运行时") {
                LabeledContent("Steer 版本", value: model.versions.helper)
                LabeledContent("核心版本", value: model.versions.singBox)
                LabeledContent("运行状态", value: model.runtime.healthy ? "正常" : (model.runtime.generationID.isEmpty ? "未运行" : "异常"))
                LabeledContent("上次应用", value: model.runtime.lastApply.map { $0.result.ok ? "成功" : "失败" } ?? "—")
                LabeledContent("规则数据版本", value: model.versions.geoVersion)
                LabeledContent("规则总数", value: model.versions.geoRuleCount?.formatted() ?? "—")
                LabeledContent("GeoSite 规则集", value: model.geositeNames.count.formatted())
                LabeledContent("GeoIP 分类", value: model.geoipNames.count.formatted())
            }
            Section("存储路径") {
                pathRow("配置", "/Library/Application Support/Steer/config/config.json")
                pathRow("运行目录", "/Library/Application Support/Steer/run")
                pathRow("状态目录", "/Library/Application Support/Steer/state")
                pathRow("规则数据", "/Library/Application Support/Steer/geodata-seed")
                pathRow("日志", "/Library/Logs/Steer")
            }
            Section("授权") {
                Text("首次安装系统组件需要管理员授权；日常保存和应用配置不再重复请求授权。")
                    .foregroundStyle(.secondary)
            }
        }
        .listStyle(.inset)
        .confirmationDialog("卸载 Steer 系统组件？", isPresented: $showingUninstall, titleVisibility: .visible) {
            Button("卸载并保留用户数据", role: .destructive) {
                model.uninstallSystemComponents(removeUserData: false)
            }
            Button("同时删除用户数据…", role: .destructive) {
                DispatchQueue.main.async { showingDeleteUserData = true }
            }
            Button("取消", role: .cancel) {}
        } message: {
            Text("将停止 Steer 服务并删除系统组件；用户配置、运行状态和日志默认保留。")
        }
        .alert("同时删除用户数据？", isPresented: $showingDeleteUserData) {
            Button("永久删除配置、状态与日志", role: .destructive) {
                model.uninstallSystemComponents(removeUserData: true)
            }
            Button("取消", role: .cancel) {}
        } message: {
            Text("这会额外删除本机保存的配置、状态与日志，无法从 Steer 恢复。")
        }
    }

    private func pathRow(_ title: String, _ path: String) -> some View {
        LabeledContent(title) {
            Text(path)
                .font(.callout.monospaced())
                .textSelection(.enabled)
        }
    }

    private func componentSymbol(_ state: SystemComponentFact.State) -> String {
        switch state {
        case .ready: return "checkmark.circle.fill"
        case .inactive: return "pause.circle"
        case .outdated: return "arrow.down.circle.fill"
        case .missing: return "questionmark.circle"
        case .invalid: return "xmark.octagon.fill"
        }
    }

    private func componentColor(_ state: SystemComponentFact.State) -> Color {
        switch state {
        case .ready: return .green
        case .inactive: return .secondary
        case .outdated: return .orange
        case .missing, .invalid: return .red
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
        case .configuration: return "高级配置"
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
        case .general: return "配置"
        case .configuration: return "JSON"
        case .nodes: return "节点"
        case .routes: return "路由"
        case .dns: return "DNS Profiles"
        case .rules: return "顺序规则"
        case .subscriptions: return "订阅"
        case .proxies: return "本地代理"
        case .diagnostics: return "诊断"
        case .settings: return "系统"
        }
    }

    var subtitle: String {
        switch self {
        case .overview: return "配置概览与服务运行状态"
        case .general: return "运行、连通性探测、DNS 缓存与启动解析设置"
        case .configuration: return "高级编辑与校验；所有可视化页面共享同一份工作副本"
        case .nodes: return "手动节点可编辑，订阅节点保持只读"
        case .routes: return "管理直连、拒绝与单节点出口链路"
        case .dns: return "上游解析器与每条规则的独立 DNS 路径"
        case .rules: return "从上到下严格匹配，Default 必须位于最后"
        case .subscriptions: return "管理订阅源、节点更新与失效节点清理"
        case .proxies: return "本机 SOCKS、HTTP 与 Mixed 入口"
        case .diagnostics: return "连通性报告、配置校验、应用结果与系统日志"
        case .settings: return "系统组件、版本与存储路径"
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
