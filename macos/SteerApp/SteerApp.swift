// SPDX-License-Identifier: GPL-3.0-or-later

import AppKit
import SwiftUI

@main
struct SteerApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        WindowGroup("Steer") {
            ContentView(model: model)
                .frame(minWidth: 960, minHeight: 640)
        }
        .commands {
            CommandGroup(replacing: .appInfo) {
                Button("Validate") { model.validate() }
                    .keyboardShortcut("v", modifiers: [.command, .option])
            }
        }
        MenuBarExtra("Steer", systemImage: model.runtime.healthy ? "checkmark.shield" : "shield") {
            Button(model.runtime.healthy ? "Disable" : "Enable") { model.toggleEnabled() }
            Divider()
            Button("Open Steer") { model.selectedPage = .overview }
            Button("Quit") { NSApplication.shared.terminate(nil) }
        }
    }
}
