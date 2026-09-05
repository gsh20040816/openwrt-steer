// SPDX-License-Identifier: GPL-3.0-or-later

import AppKit
import SwiftUI

@MainActor
final class SteerAppDelegate: NSObject, NSApplicationDelegate {
    weak var model: AppModel?

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard let model else { return .terminateNow }
        guard !model.isBusy else {
            model.message = "当前操作尚未完成，已取消退出"
            return .terminateCancel
        }
        guard model.pendingDraftAction == nil else { return .terminateCancel }
        guard model.isDirty else { return .terminateNow }
        guard model.beginTerminationGuard(reply: { allow in
            sender.reply(toApplicationShouldTerminate: allow)
        }) else {
            return .terminateCancel
        }

        sender.activate(ignoringOtherApps: true)
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = model.draftGuardTitle
        alert.informativeText = model.draftGuardExplanation
        let saveButton = alert.addButton(withTitle: "保存")
        saveButton.isEnabled = model.canSaveForPendingDraftAction
        alert.addButton(withTitle: "丢弃")
        alert.addButton(withTitle: "取消")

        switch alert.runModal() {
        case .alertFirstButtonReturn:
            model.resolveDraftGuard(.save)
        case .alertSecondButtonReturn:
            DispatchQueue.main.async { model.resolveDraftGuard(.discard) }
        default:
            DispatchQueue.main.async { model.resolveDraftGuard(.cancel) }
        }
        return .terminateLater
    }
}

@main
@MainActor
struct SteerApp: App {
    @NSApplicationDelegateAdaptor(SteerAppDelegate.self) private var appDelegate
    @StateObject private var model = AppModel()

    var body: some Scene {
        WindowGroup("Steer", id: "main") {
            ContentView(model: model)
                .frame(minWidth: 1_080, minHeight: 700)
                .onAppear { appDelegate.model = model }
                .task { model.loadInitialState() }
        }
        .defaultSize(width: 1_180, height: 760)
        .commands {
            CommandGroup(replacing: .appInfo) {
                Button("校验配置") { model.validate() }
                    .keyboardShortcut("v", modifiers: [.command, .option])
            }
        }
        MenuBarExtra("Steer", systemImage: model.runtime.healthy ? "checkmark.shield" : "shield") {
            MenuBarContent(model: model)
                .onAppear { appDelegate.model = model }
        }
    }
}

private struct MenuBarContent: View {
    @ObservedObject var model: AppModel
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        Button(model.savedEnabled ? "停用 Steer" : "启用 Steer") {
            model.setEnabledAndApply(!model.savedEnabled)
        }
        .disabled(!model.canToggleEnabled)
        Divider()
        DraftActionButtons(model: model)
        Button("刷新状态") { model.refreshStatus() }
            .disabled(model.isBusy || model.pendingDraftAction != nil)
        Divider()
        Button("打开 Steer") {
            model.selectedPage = .overview
            openWindow(id: "main")
            NSApplication.shared.activate(ignoringOtherApps: true)
        }
        Button("退出") { NSApplication.shared.terminate(nil) }
    }
}
