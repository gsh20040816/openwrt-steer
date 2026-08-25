// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "SteerMacOS",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "SteerApp", targets: ["SteerApp"]),
    ],
    targets: [
        .executableTarget(
            name: "SteerApp",
            dependencies: [],
            path: "SteerApp",
            exclude: ["Info.plist"]
        ),
    ]
)
