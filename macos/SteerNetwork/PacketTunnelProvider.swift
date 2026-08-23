// SPDX-License-Identifier: GPL-3.0-or-later

import NetworkExtension

/// The provider owns the NetworkExtension TUN settings. It deliberately does
/// not install NEDNSSettings; DNSProxyProvider is the sole DNS capture owner.
final class PacketTunnelProvider: NEPacketTunnelProvider {
    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "198.18.0.2")
        let ipv4 = NEIPv4Settings(addresses: ["198.18.0.1"], subnetMasks: ["255.255.255.252"])
        ipv4.includedRoutes = [NEIPv4Route.default()]
        settings.ipv4Settings = ipv4
        settings.dnsSettings = nil

        // The Libbox command server and generation handoff are intentionally
        // left behind the coordinator contract until a signed Mac target is
        // available. Do not call completionHandler(nil) before Libbox owns the
        // packet flow in the real implementation.
        setTunnelNetworkSettings(settings) { error in
            completionHandler(error ?? PacketTunnelProviderError.runtimePending)
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        completionHandler()
    }
}

private enum PacketTunnelProviderError: Error {
    case runtimePending
}
