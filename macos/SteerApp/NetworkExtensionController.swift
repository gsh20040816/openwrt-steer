// SPDX-License-Identifier: GPL-3.0-or-later

import Foundation
import NetworkExtension

/// Owns only the OS-managed provider profiles. Generation preparation and
/// status publication stay in the Go macOS adapter so both providers receive
/// one digest and the app never edits a running generation in place.
@MainActor
final class NetworkExtensionController {
    struct BundleIdentifiers {
        let packetTunnel: String
        let dnsProxy: String
    }

    private let bundleIdentifiers: BundleIdentifiers

    init(bundleIdentifiers: BundleIdentifiers) {
        self.bundleIdentifiers = bundleIdentifiers
    }

    func installPacketTunnel(generationID: String) async throws -> NETunnelProviderManager {
        let managers = try await loadTunnelManagers()
        let manager = managers.first ?? NETunnelProviderManager()
        let configuration = NETunnelProviderProtocol()
        configuration.providerBundleIdentifier = bundleIdentifiers.packetTunnel
        configuration.serverAddress = "Steer"
        configuration.providerConfiguration = ["generation_id": generationID]
        manager.protocolConfiguration = configuration
        manager.localizedDescription = "Steer Packet Tunnel"
        manager.isEnabled = true
        try await save(manager)
        try await load(manager)
        return manager
    }

    func installDNSProxy(generationID: String) async throws -> NEDNSProxyManager {
        let manager = NEDNSProxyManager.shared()
        try await load(manager)
        let configuration = NEDNSProxyProviderProtocol()
        configuration.providerBundleIdentifier = bundleIdentifiers.dnsProxy
        configuration.providerConfiguration = ["generation_id": generationID]
        manager.providerProtocol = configuration
        manager.localizedDescription = "Steer DNS Proxy"
        manager.isEnabled = true
        try await save(manager)
        try await load(manager)
        return manager
    }

    func disable() async throws {
        let tunnelManagers = try await loadTunnelManagers()
        for manager in tunnelManagers where manager.isEnabled {
            manager.isEnabled = false
            try await save(manager)
        }
        let dnsManager = NEDNSProxyManager.shared()
        try await load(dnsManager)
        if dnsManager.isEnabled {
            dnsManager.isEnabled = false
            try await save(dnsManager)
        }
    }

    private func loadTunnelManagers() async throws -> [NETunnelProviderManager] {
        try await withCheckedThrowingContinuation { continuation in
            NETunnelProviderManager.loadAllFromPreferences { managers, error in
                if let error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume(returning: managers ?? [])
                }
            }
        }
    }

    private func load(_ manager: NETunnelProviderManager) async throws {
        try await withCheckedThrowingContinuation { continuation in
            manager.loadFromPreferences { error in
                if let error { continuation.resume(throwing: error) }
                else { continuation.resume() }
            }
        }
    }

    private func save(_ manager: NETunnelProviderManager) async throws {
        try await withCheckedThrowingContinuation { continuation in
            manager.saveToPreferences { error in
                if let error { continuation.resume(throwing: error) }
                else { continuation.resume() }
            }
        }
    }

    private func load(_ manager: NEDNSProxyManager) async throws {
        try await withCheckedThrowingContinuation { continuation in
            manager.loadFromPreferences { error in
                if let error { continuation.resume(throwing: error) }
                else { continuation.resume() }
            }
        }
    }

    private func save(_ manager: NEDNSProxyManager) async throws {
        try await withCheckedThrowingContinuation { continuation in
            manager.saveToPreferences { error in
                if let error { continuation.resume(throwing: error) }
                else { continuation.resume() }
            }
        }
    }
}
