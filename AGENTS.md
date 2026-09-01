# Repository Engineering Rules

These instructions apply to the entire repository. Every contributor or coding
agent must follow them for every production-code change.

## Core requirement

Do not declare a change complete based only on correctness tests. Every change
must be evaluated for performance, allocations, concurrency safety, memory
safety, and ownership/lifetime correctness. Documentation-only and test-only
changes may state that runtime benchmarks and memory checks are not applicable,
but must still run the relevant formatting and validation checks.

## Before changing code

1. Identify the production execution path affected by the change.
2. Check the working tree and preserve unrelated user changes.
3. Find an existing representative benchmark. If none exists, add one before
   modifying production code.
4. Record a reproducible baseline using at least five benchmark samples:

   ```sh
   go test <affected-package> -run '^$' -bench '<benchmark-regexp>' -benchmem -count=5
   ```

5. Benchmark realistic payload sizes and failure paths when they can materially
   affect production. Do not benchmark only a helper if production uses it
   through a materially different path.

## Implementation constraints

- Preserve exported APIs and observable behavior unless the task explicitly
  authorizes a breaking change.
- Treat values received from JavaScript, HTTP, and CGO as untrusted.
- Keep response/request body sizes bounded before allocating or copying.
- Do not retain Go pointers in C/V8 memory or allow Go-backed memory to outlive
  the call that owns it.
- Document and test ownership whenever a value crosses the Go/V8 or Go/CGO
  boundary. Verify which side owns isolates, contexts, callbacks, buffers,
  timers, response bodies, and goroutines, and when each is released.
- Close bodies and stop timers/tickers on every success, error, cancellation,
  and early-return path.
- Ensure goroutines have a bounded lifetime and a shutdown/cancellation path.
- Do not access an isolate or context concurrently unless the v8go API
  explicitly guarantees that use is safe.
- Avoid unbounded allocation based on `Content-Length`, JavaScript strings,
  header values, or other external metadata.
- Preserve unknown-length/chunked behavior and HTTP bodyless semantics such as
  `HEAD`, `1xx`, `204`, and `304` when changing fetch code.
- Add regression tests for boundary sizes: zero, exact limit, limit plus one,
  unknown size, malformed input, early EOF, and repeated/concurrent cleanup as
  applicable.

## Required verification

Run all checks below that the environment supports. A production-code change is
not complete until the applicable checks pass.

```sh
gofmt -w <changed-go-files>
go vet ./...
go test ./... -count=1
go test -race ./... -count=1
go test -gcflags=all=-d=checkptr=2 ./... -count=1
go test <affected-package> -run '^$' -bench '<benchmark-regexp>' -benchmem -count=5
git diff --check
```

On macOS, this repository's v8go linker may require:

```sh
CGO_LDFLAGS='-framework CoreFoundation' go test ./... -count=1
```

Use a writable `GOCACHE` such as `/tmp/v8go-polyfills-gocache` when the default
cache is unavailable. If tests need a localhost listener, request the required
sandbox permission rather than skipping them.

For changes involving unsafe code, CGO pointer ownership, V8 object lifetime,
or native buffers, also run the strongest sanitizer-enabled build supported by
the toolchain (for example ASan) and exercise the affected integration path.
Never claim sanitizer coverage if the installed v8go/native dependency was not
built with compatible sanitizer instrumentation.

## Benchmark acceptance

- Compare before and after using the same machine, Go version, environment,
  benchmark inputs, and command.
- Report `ns/op`, `B/op`, and `allocs/op`; include throughput or peak retained
  memory when relevant.
- Use at least five samples. Prefer `benchstat` when available; otherwise report
  medians and explicitly note measurement noise.
- Do not recommend or keep a performance optimization based on a single run.
- A performance-oriented change must show a repeatable production-relevant
  improvement without a material regression in normal, error, or concurrent
  paths.
- If results are neutral or worse, revert the optimization unless it provides a
  separately stated correctness or security benefit.
- Keep representative benchmarks in the repository to prevent regressions.

## Completion report

Every final handoff for a production-code change must include:

- affected production path and compatibility assessment;
- benchmark command and before/after results;
- test, race detector, checkptr, vet, and formatting status;
- ownership/lifetime risks reviewed;
- any skipped or unsupported check, the exact reason, and the residual risk.

Do not use “all tests passed” when only a subset ran. Do not hide failures caused
by sandboxing, linker setup, missing sanitizer builds, or unavailable tooling.
