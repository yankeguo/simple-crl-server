# Project Context

## Purpose
A simple HTTP server that generates and serves Certificate Revocation Lists (CRL) for Public Key Infrastructure (PKI) systems. The server enables certificate revocation management with hot-reload support for Kubernetes deployments, automatic caching for performance, and graceful shutdown handling for production environments.

## Tech Stack
- **Go 1.25** - Primary language with standard library only (no external dependencies)
- **Docker** - Multi-stage builds using Alpine Linux for minimal container images
- **Kubernetes** - Deployment-ready with ConfigMap/Secret hot-reload support
- **GitHub Actions** - Automated release workflow for Docker image publishing
- **OpenSSL** - Used by Makefile for generating test certificates

## Project Conventions

### Code Style
- **Go conventions**: Follow standard Go formatting (gofmt)
- **Package structure**: Single `main` package in `main.go` for simplicity
- **Error handling**: Always wrap errors with context using `fmt.Errorf` with `%w`
- **Logging**: Use standard `log` package for server events
- **Naming**: PascalCase for exported, camelCase for internal
- **Constants**: Define at top of file using `const` block
- **File permissions**: 0755 for directories, 0644 for files

### Architecture Patterns
- **Single-file architecture**: All server logic in `main.go` (~410 lines)
- **Caching pattern**: Double-checked locking with `sync.RWMutex`
- **Hot-reload design**: Reload certificate/key/revocation list on each CRL generation
- **Graceful shutdown**: Context-based timeout (30 seconds) with SIGINT/SIGTERM handling
- **HTTP handlers**: `http.ServeMux` with separate handler functions
- **File structure**:
  - `./tls/tls.crt` - CA certificate (PEM)
  - `./tls/tls.key` - CA private key (PEM: PKCS8, PKCS1, or EC)
  - `./conf/list.txt` - Revocation list
  - `./temp/` - CRL cache directory

### Testing Strategy
- Currently **no automated tests** - area for improvement
- Manual testing via Makefile targets:
  - `make test-cert` - Generate test CA certificate/key
  - `make test-list` - Create example revocation list
  - `make setup` - Run both setup tasks
  - `make run` - Build and run server locally
- Manual verification: `curl http://localhost:8080/ -o revocation.crl && openssl crl -inform DER -in revocation.crl -text -noout`

### Git Workflow
- **Branching strategy**: Single `main` branch
- **Commit convention**: Conventional commits (based on history)
  - `feat:` - New features
  - `fix:` - Bug fixes
  - `ci(github):` - CI/CD changes
  - `feat(server):` - Server changes
  - `fix(build):` - Build-related fixes
- **Release workflow**:
  - Push to `main` → publishes `:latest` Docker image
  - Tag push (e.g., `v1.2.3`) → publishes versioned tags (`1.2.3`, `1.2`, `1`)
- **Semantic versioning**: Follow SemVer (MAJOR.MINOR.PATCH)

## Domain Context

### Certificate Revocation Lists (CRL)
- **CRL format**: DER-encoded ASN.1 (application/pkix-crl)
- **CRL number**: Current Unix timestamp used as CRL number
- **Cache duration**: 1 hour (configurable via `cacheDuration` constant)
- **Revocation list format**: `[serial_number_hex]:[epoch_timestamp]:[reason_code]`
  - Example: `1A2B3C4D5E6F:1704067200:1`
- **Revocation reason codes** (RFC 5280):
  - 0: Unspecified
  - 1: Key Compromise
  - 2: CA Compromise
  - 3: Affiliation Changed
  - 4: Superseded
  - 5: Cessation of Operation
  - 6: Certificate Hold
  - 8: Remove from CRL
  - 9: Privilege Withdrawn
  - 10: AA Compromise

### Key Formats Supported
- **PKCS#8** - Preferred, tried first
- **PKCS#1** - RSA keys
- **EC** - Elliptic Curve keys
- All in PEM format

## Important Constraints

### Technical Constraints
- **Port**: Hardcoded to 8080
- **Cache duration**: 1 hour (hardcoded, not configurable)
- **File paths**: Fixed relative paths (`./tls`, `./conf`, `./temp`)
- **No external dependencies**: Uses only Go standard library
- **Single binary**: No configuration files or environment variables

### Operational Constraints
- **Cache persistence**: Survives restarts via disk cache in `./temp/`
- **Hot-reload latency**: Changes take effect within cache duration (max 1 hour)
- **Graceful shutdown timeout**: 30 seconds maximum
- **HTTP**: Plaintext only (no HTTPS/TLS termination)

### Kubernetes Constraints
- **Health check**: `/healthz` endpoint returns "OK\n" with 200 status
- **Volume mounts**: tls (ro), conf (ro), temp (rw)
- **Security**: Runs as non-root user (UID 1000)
- **Platforms**: linux/amd64, linux/arm64

## External Dependencies
**None** - The project uses only Go standard library modules:
- `crypto` - Cryptographic primitives
- `crypto/x509` - Certificate and CRL generation
- `net/http` - HTTP server
- `os`, `syscall` - Signal handling and file I/O
- `sync` - Concurrency primitives
- `time` - Time handling

### Build Dependencies
- **Go 1.25** - Toolchain for building
- **Docker** - Container image building (multi-stage)
- **Alpine Linux** - Base image for runtime
- **ca-certificates** - Installed for HTTPS support in healthcheck

### Development Dependencies
- **OpenSSL** - Used by Makefile for test certificate generation
