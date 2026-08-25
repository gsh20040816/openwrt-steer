// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "SteerMacOS",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "SteerApp", targets: ["SteerApp"]),
        .library(name: "SteerAgent", targets: ["SteerAgent"]),
    ],
    targets: [
        .executableTarget(
            name: "SteerApp",
            dependencies: ["SteerAgent"],
            path: "SteerApp",
            exclude: ["Info.plist"]
        ),
        .target(
            name: "SteerAgent",
            path: "SteerAgent",
            exclude: ["com.gsh20040816.steer.agent.plist"]
        ),
    ]
)
