// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "SteerMacOS",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "SteerApp", targets: ["SteerApp"]),
        .library(name: "SteerNetwork", targets: ["SteerNetwork"]),
    ],
    targets: [
        .executableTarget(
            name: "SteerApp",
            path: "SteerApp"
        ),
        .target(
            name: "SteerNetwork",
            path: "SteerNetwork"
        ),
    ]
)
