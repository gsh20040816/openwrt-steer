// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "SteerMacOS",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "SteerApp", targets: ["SteerApp"]),
        .library(name: "SteerAgent", targets: ["SteerAgent"]),
        .library(name: "SteerNetwork", targets: ["SteerNetwork"]),
    ],
    targets: [
        .executableTarget(
            name: "SteerApp",
            dependencies: ["SteerAgent"],
            path: "SteerApp"
        ),
        .target(
            name: "SteerAgent",
            path: "SteerAgent"
        ),
        .target(
            name: "SteerNetwork",
            path: "SteerNetwork"
        ),
    ]
)
