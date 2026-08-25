// SPDX-License-Identifier: GPL-3.0-or-later

import SwiftUI

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
                .disabled(model.isBusy)
                Button { model.validate() } label: {
                    Label("校验", systemImage: "checkmark.shield")
                }
                .disabled(model.isBusy || model.draftSyntaxError != nil)
                Button { model.apply() } label: {
                    Label("应用", systemImage: "bolt.fill")
                }
                .disabled(model.isBusy || model.draftSyntaxError != nil)
            }
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
            Section("状态") {
                sidebarRow(.overview)
            }
            Section("配置") {
                sidebarRow(.general)
                sidebarRow(.nodes, count: model.itemCount(for: "nodes"))
                sidebarRow(.routes, count: model.itemCount(for: "routes"))
                sidebarRow(.dns, count: model.itemCount(for: "dns_profiles"))
                sidebarRow(.proxies, count: model.itemCount(for: "local_proxies"))
                sidebarRow(.rules, count: model.itemCount(for: "rules"))
            }
            Section("服务") {
                sidebarRow(.subscriptions, count: model.itemCount(for: "subscriptions"))
                sidebarRow(.diagnostics)
                sidebarRow(.settings)
            }
            Section("高级") {
                sidebarRow(.configuration)
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
                    .disabled(model.isBusy)
                    ControlGroup {
                        Button("校验") { model.validate() }
                            .disabled(model.draftSyntaxError != nil)
                        Button("应用") { model.apply() }
                            .buttonStyle(.borderedProminent)
                            .disabled(model.draftSyntaxError != nil)
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
            pipelineStep(value: model.itemCount(for: "routes"), title: "路由", subtitle: "Direct / Block / Single", symbol: "arrow.triangle.branch")
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
                    Button("保存") { model.saveDraft() }
                        .disabled(!model.isDirty)
                    Button("校验") { model.validate() }
                        .disabled(model.draftSyntaxError != nil)
                    Button("保存并应用") { model.apply() }
                        .buttonStyle(.borderedProminent)
                        .disabled(model.draftSyntaxError != nil)
                }
                .disabled(model.isBusy)
            }
            VStack(alignment: .leading, spacing: 8) {
                Label("Canonical JSON", systemImage: "doc.plaintext")
                    .font(.headline)
                Divider()
                TextEditor(text: Binding(
                    get: { model.rawJSON },
                    set: { model.rawJSON = $0; model.markDirty() }
                ))
                .font(.system(.body, design: .monospaced))
                .textSelection(.enabled)
                .disabled(model.isBusy)
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

struct DraftCollectionView: View {
    @ObservedObject var model: AppModel
    let descriptor: DraftCollectionDescriptor
    @State private var selection: DraftItem.ID?
    @State private var editorTarget: DraftEditorTarget?
    @State private var pendingDeletion: DraftItem?
    @State private var actionError = ""
    @State private var nodeImportPresented = false

    private var items: [DraftItem] { model.draftItems(for: descriptor.key) }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label(descriptor.title, systemImage: descriptor.symbol)
                    .font(.headline)
                Text("\(items.count) 项")
                    .foregroundStyle(.secondary)
                Spacer()
                Button { editSelected() } label: {
                    Label("编辑", systemImage: "square.and.pencil")
                }
                .disabled(model.isBusy || selectedItem == nil || selectedItem?.subscriptionOwned == true)
                if descriptor.key == "nodes" {
                    Button { nodeImportPresented = true } label: {
                        Label("导入", systemImage: "square.and.arrow.down")
                    }
                    .disabled(model.isBusy)
                }
                Button { addItem() } label: {
                    Label(descriptor.addLabel, systemImage: "plus")
                }
                .buttonStyle(.borderedProminent)
                .disabled(model.isBusy)
            }

            Table(of: DraftItem.self, selection: $selection) {
                TableColumn("状态") { item in
                    Toggle("", isOn: Binding(
                        get: { item.enabled },
                        set: { model.setDraftItemEnabled(in: descriptor.key, at: item.index, enabled: $0) }
                    ))
                    .labelsHidden()
                    .toggleStyle(.switch)
                    .controlSize(.small)
                    .disabled(item.subscriptionOwned || (descriptor.key == "routes" && item.kind == "direct"))
                }
                .width(58)
                TableColumn("名称") { item in
                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(item.title).fontWeight(.medium)
                        }
                        Spacer(minLength: 0)
                    }
                }
                TableColumn("类型") { item in
                    Text(item.kind.isEmpty ? "—" : item.kind.uppercased())
                        .font(.caption.weight(.medium))
                }
                .width(min: 80, ideal: 110)
                TableColumn("详情") { item in
                    Text(item.detail.isEmpty ? "—" : item.detail)
                        .font(.callout.monospaced())
                        .lineLimit(1)
                        .foregroundStyle(.secondary)
                }
                TableColumn("顺序") { item in
                    HStack(spacing: 7) {
                        if descriptor.ordered {
                            Text(item.index + 1, format: .number)
                                .font(.caption.monospacedDigit())
                                .foregroundStyle(.secondary)
                        }
                        Image(systemName: isPinned(item) ? "pin.fill" : "line.3.horizontal")
                            .foregroundStyle(.secondary)
                    }
                    .frame(maxWidth: .infinity)
                    .help(isPinned(item) ? "Default 固定在最后" : "拖动调整顺序")
                    .accessibilityLabel(isPinned(item) ? "\(item.title) 固定在最后" : "拖动 \(item.title) 调整顺序")
                }
                .width(78)
                TableColumn("操作") { item in
                    HStack(spacing: 8) {
                        Button { edit(item) } label: {
                            Image(systemName: "square.and.pencil")
                        }
                        .buttonStyle(.borderless)
                        .disabled(item.subscriptionOwned)
                        Button(role: .destructive) {
                            requestDeletion(item)
                        } label: {
                            Image(systemName: "trash")
                        }
                        .buttonStyle(.borderless)
                        .disabled(item.subscriptionOwned)
                    }
                }
                .width(70)
            } rows: {
                ForEach(items) { item in
                    TableRow(item)
                        .itemProvider {
                            isPinned(item) ? nil : NSItemProvider(object: item.id as NSString)
                        }
                }
                .dropDestination(for: String.self) { destination, identifiers in
                    moveItems(identifiers, to: destination)
                }
            }
            .disabled(model.isBusy)
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
                    Button("上移") { model.moveDraftItem(in: descriptor.key, at: item.index, offset: -1) }
                        .disabled(item.index == 0 || isPinned(item))
                    Button("下移") { model.moveDraftItem(in: descriptor.key, at: item.index, offset: 1) }
                        .disabled(!canMoveDown(item))
                    Divider()
                    Button("编辑") { edit(item) }
                        .disabled(item.subscriptionOwned)
                    Button("删除", role: .destructive) {
                        requestDeletion(item)
                    }
                    .disabled(item.subscriptionOwned)
                }
            } primaryAction: { selected in
                if let id = selected.first, let item = items.first(where: { $0.id == id }) {
                    edit(item)
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
            items.firstIndex(where: { $0.id == identifier })
        })
        guard !sources.isEmpty, !sources.contains(where: { isPinned(items[$0]) }) else { return }
        let resolvedDestination: Int
        if descriptor.key == "rules", let defaultIndex = items.firstIndex(where: isPinned) {
            resolvedDestination = min(destination, defaultIndex)
        } else {
            resolvedDestination = destination
        }
        model.moveDraftItem(in: descriptor.key, from: sources, to: resolvedDestination)
    }

    private func isPinned(_ item: DraftItem) -> Bool {
        descriptor.key == "rules" && item.kind.caseInsensitiveCompare("default") == .orderedSame
    }

    private func canMoveDown(_ item: DraftItem) -> Bool {
        guard !isPinned(item), item.index < items.count - 1 else { return false }
        return !isPinned(items[item.index + 1])
    }
}

struct DiagnosticsView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        List {
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
        }
        .listStyle(.inset)
        .task {
            if model.diagnosticsLog.isEmpty { model.refreshLogs() }
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
            Section("版本与运行时") {
                LabeledContent("Canonical schema", value: model.draftSchemaVersion == 0 ? "—" : String(model.draftSchemaVersion))
                LabeledContent("Backend", value: "steer-macos \(model.versions.helper)")
                LabeledContent("数据面", value: "sing-box \(model.versions.singBox) · TUN")
                LabeledContent("LaunchDaemon", value: "com.steer.steer")
            }
            Section("存储路径") {
                pathRow("配置", "/Library/Application Support/Steer/config/config.json")
                pathRow("运行目录", "/Library/Application Support/Steer/run")
                pathRow("状态目录", "/Library/Application Support/Steer/state")
                pathRow("Geo Seed", "/Library/Application Support/Steer/geodata-seed")
                pathRow("日志", "/Library/Logs/Steer")
            }
            Section("授权") {
                Text("启动、刷新状态与校验无需授权；只有保存和应用配置时才通过 macOS 管理员授权调用 root-owned steer-macos helper。")
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
        case .routes: return "Direct、Block 与 Single Route 的确定性出口关系"
        case .dns: return "上游解析器与每条规则的独立 DNS 路径"
        case .rules: return "从上到下严格匹配，Default 必须位于最后"
        case .subscriptions: return "订阅源、节点同步与 stale 清理"
        case .proxies: return "本机 SOCKS、HTTP 与 Mixed 入口"
        case .diagnostics: return "配置校验、LaunchDaemon 状态与最近消息"
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
