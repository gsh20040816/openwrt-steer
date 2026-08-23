module github.com/gsh20040816/steer/macos/bridge

go 1.26

// Keep the embedded Apple runtime on the reviewed sing-box baseline. The
// actual Libbox/XCFramework build is intentionally deferred to a macOS host.
require (
	github.com/gsh20040816/steer/go v0.0.0
	github.com/sagernet/sing-box v1.13.19
)

replace github.com/gsh20040816/steer/go => ../../go
