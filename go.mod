module github.com/tommie/v8go-polyfills

go 1.26

require github.com/tommie/v8go v0.36.0

require (
	github.com/tommie/v8go/deps/android_amd64 v0.0.0-20261001103105-d7df33db7fe1 // indirect
	github.com/tommie/v8go/deps/android_arm64 v0.0.0-20261001103105-d7df33db7fe1 // indirect
	github.com/tommie/v8go/deps/darwin_amd64 v0.0.0-20261001103105-d7df33db7fe1 // indirect
	github.com/tommie/v8go/deps/darwin_arm64 v0.0.0-20261001103105-d7df33db7fe1 // indirect
	github.com/tommie/v8go/deps/linux_amd64 v0.0.0-20261001103105-d7df33db7fe1 // indirect
	github.com/tommie/v8go/deps/linux_arm64 v0.0.0-20261001103105-d7df33db7fe1 // indirect
)

retract [v0.1.0, v0.3.0]
