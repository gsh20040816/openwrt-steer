// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation
import Combine
import ServiceManagement

@MainActor
public final class AgentController: ObservableObject {
    @Published public private(set) var status: SMAppService.Status

    private let service: SMAppService

    public init(plistName: String = "com.steer.steer.agent.plist") {
        service = SMAppService.agent(plistName: plistName)
        status = service.status
    }

    public func refresh() {
        status = service.status
    }

    public func register() throws {
        try service.register()
        refresh()
    }

    public func unregister() throws {
        try service.unregister()
        refresh()
    }
}
