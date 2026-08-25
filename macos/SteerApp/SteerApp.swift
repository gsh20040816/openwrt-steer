// SPDX-License-Identifier: GPL-3.0-or-later

import AppKit
import SwiftUI

@main
@MainActor
struct SteerApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        WindowGroup("Steer", id: "main") {
            ContentView(model: model)
                .frame(minWidth: 1_080, minHeight: 700)
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
        }
    }
}

private struct MenuBarContent: View {
    @ObservedObject var model: AppModel
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        Button(model.draftEnabled ? "停用 Steer" : "启用 Steer") {
            model.setEnabledAndApply(!model.draftEnabled)
        }
        .disabled(model.isBusy)
        Button("刷新状态") { model.refreshStatus() }
        Divider()
        Button("打开 Steer") {
            model.selectedPage = .overview
            openWindow(id: "main")
            NSApplication.shared.activate(ignoringOtherApps: true)
        }
        Button("退出") { NSApplication.shared.terminate(nil) }
    }
}
