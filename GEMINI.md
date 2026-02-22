# Gemini Project Mandates: Go-Docker Sidecar

This project follows a specific architectural pattern for building lightweight, rootless Go-based sidecar containers. Adhere strictly to these mandates to maintain design consistency and security.

## 1. Architectural Mandates

### Go Entrypoint (`main.go`)
- **Single Process Management**: The Go entrypoint must be the main process (`PID 1`).
- **Event-Driven & Polling**: The sidecar should expose HTTP endpoints (e.g., `/track`) to be triggered by external events, and use goroutines with tickers for background synchronization or polling.
- **Graceful Shutdown**: Use `context.Context`, `sync.WaitGroup`, and signal intercepts to ensure all background workers and the HTTP server shut down gracefully upon receiving termination signals.
- **Environment Variables**: Use custom helper functions (like `getEnv`, `getEnvInt`, `getEnvBool`) to handle environment-driven configuration with fallback defaults.

### Containerization (`Dockerfile`)
- **Multi-Stage Builds**:
  - Always use a `builder` stage for compiling the Go entrypoint.
  - Always use a `final` stage based on `gcr.io/distroless/cc-debian12`, `scratch`, or similar minimal, shell-less images.
- **Rootless Execution**: The final image **MUST** run as a non-root user (e.g., `USER nonroot:nonroot`).
- **Static Binaries**: Compile the Go entrypoint with `CGO_ENABLED=0` to ensure compatibility with minimal base images.

## 2. Engineering Standards

### Testing & Validation
- **Empirical Reproduction**: Before fixing a bug, reproduce it with a test case in `main_test.go`.
- **Mocking**: Mock external APIs (like qBittorrent or Ntfy server) when testing to avoid requiring real network dependencies.
- **No Side Effects**: Unit tests must not leak goroutines or rely on external network access.
- **Linting**: All changes must pass `golangci-lint` as configured in the CI pipeline.

### CI/CD Consistency
- **Workflow Patterns**: Maintain separate workflows for `PR Validation` and `CD`.
  - **PR Validation** (`pr.yml`): Runs on `pull_request` and `push` to main branches to build, test, and scan the image without pushing to a registry.
  - **CD / Release** (`build.yml`): Runs manually on `workflow_dispatch` to calculate semantic versioning, generate a changelog from merged commits, tag the release, and push the artifact to the registry.
- **Vulnerability Scanning**: Use `Trivy` in the PR validation pipeline for OS and library-level scans.
- **Version Management**:
  - Dynamically calculate semantic versions primarily from git commits since the last tag.
  - Generate a changelog exactly 1-to-1 with merged commits (1 merged commit = 1 changelog line).
  - Use the generated changelog and calculated version to automatically publish a GitHub Release.

## 3. Style and Conventions
- **Go Version**: Keep the Go version in `go.mod` and `Dockerfile` synchronized.
- **Explicit Imports**: Group standard library imports separately from third-party ones.
- **Error Handling**: Log detailed errors with context, rather than failing silently.
- **Minimalism**: Avoid adding libraries for functionality that can be achieved with the Go standard library (e.g., standard `net/http` client/server, simple timers and custom JSON decoding).

## 4. Security
- **No Shell**: Do not include `sh`, `bash`, or other shells in the final image.
- **Credentials**: Never log sensitive environment variables or credentials (e.g., `QBIT_PASS`, `NTFY_PASS`).
- **Static Analysis**: Maintain `golangci-lint` and `Trivy` as blocking gates in CI.
