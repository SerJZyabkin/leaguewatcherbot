# ADR 001: Docker Containerization for League Watcher Bot

## Status

**Accepted** - 2026-05-04

## Context

The League Watcher Bot currently runs as a standalone Go binary with manual deployment requiring:

1. **Go toolchain installation**: Go 1.22+ must be installed on deployment target
2. **Manual dependency management**: `go mod download` before each deployment
3. **Environment setup**: Environment variables must be manually configured on each system
4. **Configuration management**: `config.yaml` must be placed in the correct location relative to binary
5. **No deployment consistency**: Different environments (dev/staging/prod) may have different Go versions, system libraries, or configurations
6. **Data persistence challenges**: `pidors.json` and `log.json` must be manually backed up before redeployment

### Problems with Current Approach

- **High deployment friction**: Each new environment requires full Go setup
- **Environment drift**: Difficult to ensure dev matches prod
- **Manual scaling**: Hard to run multiple instances or deploy to cloud platforms
- **No standardized operations**: Start, stop, restart, logs all require custom scripts
- **Backup complexity**: No built-in data persistence strategy

### Requirements for Solution

1. **Simplify deployment**: Single-command setup for new environments
2. **Isolate dependencies**: No Go toolchain required on deployment targets
3. **Ensure consistency**: Identical runtime environment across dev/staging/prod
4. **Enable portability**: Easy deployment to cloud platforms (AWS ECS, Google Cloud Run, etc.)
5. **Preserve state**: Automatic data persistence for `pidors.json` and `log.json`
6. **Maintain security**: Non-root execution, minimal attack surface
7. **Keep it simple**: Avoid over-engineering for single-instance deployment

## Decision

We will **containerize the League Watcher Bot using Docker** with the following approach:

### 1. Multi-Stage Dockerfile

**Builder stage** (golang:1.22-alpine):
- Copy `go.mod` and `go.sum` first for layer caching optimization
- Download dependencies once and cache
- Build static binary with `CGO_ENABLED=0` and `-ldflags="-w -s"` (strip debug symbols)
- Verify binary exists before proceeding

**Runtime stage** (alpine:3.19):
- Minimal Alpine Linux base (~5MB)
- Install only essential runtime dependencies (ca-certificates for HTTPS, tzdata for timestamps)
- Create non-root user (UID 1000) for security
- Copy binary and default config
- Run as non-root user
- Include healthcheck (process monitoring)

**Result**: ~30-40MB final image vs ~800MB with full golang image

### 2. Docker Compose Orchestration

**Service configuration**:
- Environment variables from `.env` file (not hardcoded)
- Auto-restart policy (`unless-stopped`) for resilience
- Resource limits (256MB RAM, 0.5 CPU) to prevent runaway usage
- Log rotation (10MB max, 3 files) to prevent disk fill

**Volume strategy**:
```yaml
volumes:
  - leaguewatcher-data:/app          # Named volume for data persistence
  - ./config.yaml:/app/config.yaml:ro  # Bind mount for easy updates
```

This strategy works because:
- The bot discovers its directory via `os.Executable()` in `cmd/leaguewatcher/main.go:34-39`
- All files (binary, config, data) are in `/app`
- Named volume persists `pidors.json` and `log.json` across restarts
- Config bind mount allows updates without rebuilds
- Read-only config mount prevents accidental modification

### 3. Build Optimization

**.dockerignore**:
- Exclude VCS files, IDE files, documentation, tests
- Reduce build context from ~5MB to ~500KB
- Prevent sensitive data (`.env`) from entering image

**Layer caching**:
- Dependencies downloaded before source code copy
- Rebuilds only recompile changed code, not re-download dependencies

### 4. Security Hardening

- **Non-root user**: Container runs as UID 1000, not root
- **Read-only config**: Config file mounted `:ro` to prevent tampering
- **No exposed ports**: Bot connects outbound only (Discord, Mobalytics)
- **Minimal base**: Alpine Linux reduces attack surface
- **Static binary**: No runtime dependencies to exploit
- **Secrets in environment**: No hardcoded tokens in image

### 5. Documentation and Tooling

- **README.docker.md**: Comprehensive deployment guide
- **.env.example**: Template for required environment variables
- **ADR document**: This document explaining the decision

## Consequences

### Positive

1. **Simplified deployment**: `docker-compose up -d` vs 10+ manual steps
2. **Environment consistency**: Same image runs everywhere (local, staging, production)
3. **No toolchain required**: Deployment targets only need Docker, not Go
4. **Cloud-ready**: Works with AWS ECS, GKE, Azure Container Instances, Cloud Run
5. **Built-in data persistence**: Docker volumes handle backups/restores
6. **Standardized operations**: `docker-compose logs`, `restart`, `down` etc.
7. **Resource controls**: Memory/CPU limits prevent resource exhaustion
8. **Automatic restarts**: Bot recovers from crashes without manual intervention
9. **Developer onboarding**: New contributors can run bot in <5 minutes
10. **Testing isolation**: Can run multiple instances with different configs

### Negative

1. **Added complexity**: Developers must learn Docker basics
2. **Volume management**: Need to understand named volumes vs bind mounts
3. **Build time**: ~2 minutes for full rebuild vs ~30 seconds for `go build`
4. **Debug difficulty**: Harder to attach debugger vs native binary
5. **Disk usage**: Images and volumes consume disk space (~100MB total)
6. **Network overhead**: Minimal, but adds Docker network layer

### Neutral

1. **Image size**: ~40MB is tiny for Docker, but larger than 15MB native binary
2. **Platform lock-in**: Requires Docker, but Docker is ubiquitous
3. **Config strategy**: Could use ConfigMaps (Kubernetes) or AWS Secrets Manager in future

## Alternatives Considered

### Alternative 1: Native Binary Deployment (Current Approach)

**Pros**:
- Simple for Go developers
- Smallest artifact size (~15MB)
- Fastest startup time
- Easy to debug with dlv

**Cons**:
- Requires Go toolchain on every deployment target
- No environment consistency guarantees
- Manual dependency management
- No standardized operational tooling
- Hard to scale or deploy to cloud

**Rejection reason**: Doesn't solve the core problems of deployment friction and environment consistency.

### Alternative 2: Single-Stage Docker Build

**Pros**:
- Simpler Dockerfile (no multi-stage)
- Easier to understand for Docker beginners

**Cons**:
- Image size: ~800MB vs ~40MB (includes full Go toolchain)
- Security: More attack surface (includes build tools)
- Resource waste: Build tools unused at runtime

**Rejection reason**: Multi-stage build is Docker best practice, 20x size reduction is worth the minimal complexity.

### Alternative 3: Kubernetes Manifests (without Docker Compose)

**Pros**:
- Production-grade orchestration (auto-scaling, health checks, rolling updates)
- Better multi-instance management
- Built-in service discovery

**Cons**:
- Massive overkill for single-instance bot
- Requires Kubernetes cluster (complex to run locally)
- Much higher operational complexity
- Doesn't help with local development

**Rejection reason**: Over-engineering. Bot runs single instance per Discord server, doesn't need orchestration. Can still deploy Docker image to Kubernetes later if needed.

### Alternative 4: Systemd Service

**Pros**:
- Native Linux integration
- Automatic restarts
- Log management via journald
- No containerization overhead

**Cons**:
- Only works on Linux with systemd
- Still requires Go toolchain for builds
- No environment isolation
- Manual dependency management
- Harder to deploy to cloud

**Rejection reason**: Solves restart problem but not deployment friction or environment consistency.

## Technical Details

### Multi-Stage Build Rationale

The builder stage uses `golang:1.22-alpine` (not `golang:1.22`) because:
1. Alpine is 80% smaller than Debian-based images
2. We're building a static binary, so distribution doesn't matter
3. Faster builds due to smaller base layer

The runtime stage uses `alpine:3.19` (not `scratch`) because:
1. We need ca-certificates for HTTPS (Discord/Mobalytics)
2. We need tzdata for correct timestamps in logs
3. We need shell for healthcheck (`pgrep` command)
4. Alpine is only ~5MB larger than scratch but much more functional

### Volume Strategy Details

**Why named volume instead of bind mount for data?**

Named volumes are managed by Docker and survive:
- Container deletion (`docker-compose down`)
- Image updates (`docker-compose build && up`)
- Host machine restarts

Bind mounts would require manual directory creation and permission management.

**Why bind mount config instead of copying into image?**

Bind mounting allows config updates without rebuilds:
```bash
# Edit config
vim config.yaml

# Restart to apply
docker-compose restart
```

If config were baked into image, every config change would require:
```bash
# Rebuild image
docker-compose build

# Recreate container
docker-compose up -d
```

### Resource Limits Justification

**Memory: 256MB limit**
- Bot's baseline usage: ~50MB
- Margin for spikes: ~200MB
- Prevents memory leaks from crashing host

**CPU: 0.5 cores limit**
- Bot is I/O bound (Discord WebSocket, HTTP API calls)
- Rarely uses >10% of 1 core
- Limit prevents runaway goroutines from starving host

These limits are appropriate for:
- Small Discord server (<100 users)
- <10 tracked players
- 1-minute polling interval

Increase for larger deployments.

### Security Considerations

**Non-root user (UID 1000)**:
```dockerfile
RUN adduser -D -u 1000 -G leaguewatcher leaguewatcher
USER leaguewatcher
```

Prevents privilege escalation attacks. If container is compromised, attacker has limited permissions.

**Read-only config mount**:
```yaml
- ./config.yaml:/app/config.yaml:ro
```

Prevents bot from accidentally modifying config. Config should only change via host edits + restart.

**No exposed ports**:
Bot connects outbound to Discord and Mobalytics. No inbound connections needed, so no ports exposed. Reduces attack surface.

## Implementation

### Files Created

1. **Dockerfile** (60 lines): Multi-stage build with builder and runtime
2. **docker-compose.yml** (50 lines): Service definition, volumes, resource limits
3. **.dockerignore** (40 lines): Exclude VCS, IDE, test files
4. **.env.example** (15 lines): Environment variable template
5. **README.docker.md** (200+ lines): Comprehensive deployment guide
6. **adr/001-docker-containerization.md** (this document): Decision record

### Build and Deploy Commands

**Development**:
```bash
cp .env.example .env       # Configure
vim .env                   # Fill in tokens
docker-compose build       # Build image
docker-compose up -d       # Start bot
docker-compose logs -f     # View logs
```

**Production**:
```bash
docker build -t leaguewatcher:1.0.0 .
docker tag leaguewatcher:1.0.0 registry.example.com/leaguewatcher:1.0.0
docker push registry.example.com/leaguewatcher:1.0.0

docker run -d \
  --name leaguewatcher \
  --restart unless-stopped \
  --env-file .env \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  -v leaguewatcher-data:/app \
  --memory=256m \
  registry.example.com/leaguewatcher:1.0.0
```

## Verification

### Build Verification
```bash
docker-compose build
# Expected: <2 minutes, final image ~30-40MB

docker images | grep leaguewatcher
# Expected: leaguewatcher:latest ~40MB
```

### Runtime Verification
```bash
docker-compose up -d
docker-compose ps
# Expected: Status "Up (healthy)"

docker-compose logs | head -20
# Expected: "Starting leaguewatcher", "Config loaded"
```

### Persistence Verification
```bash
# Trigger !pidor command in Discord
docker-compose exec leaguewatcher cat /app/pidors.json
# Expected: JSON with game state

docker-compose restart
docker-compose exec leaguewatcher cat /app/pidors.json
# Expected: Same JSON (persisted)
```

## Future Considerations

### Kubernetes Deployment

If the bot needs to scale to multiple Discord servers:

1. Create Deployment manifest with replicas
2. Use ConfigMap for `config.yaml`
3. Use Secret for `BOT_DISCORD_TOKEN`
4. Use PersistentVolumeClaim for data
5. Add liveness/readiness probes (requires HTTP endpoint)

The current Docker image works with Kubernetes without modification.

### Multi-Architecture Support

To support ARM64 (Raspberry Pi, Apple Silicon):

```dockerfile
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder
ARG TARGETOS TARGETARCH

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build ...
```

Then build:
```bash
docker buildx build --platform linux/amd64,linux/arm64 -t leaguewatcher:latest .
```

### Production Logging

Current code uses `zap.NewDevelopment()` (pretty console output).

For production:
1. Add `LOG_MODE` environment variable
2. Use `zap.NewProduction()` for JSON logs
3. Ship logs to centralized logging (ELK, Splunk, CloudWatch)

### CI/CD Pipeline

GitHub Actions workflow to:
1. Run tests (`go test ./...`)
2. Build Docker image
3. Push to GitHub Container Registry
4. Deploy to production on tag push

See [README.docker.md](../README.docker.md#cicd-integration) for example workflow.

## References

- [Dockerfile best practices](https://docs.docker.com/develop/develop-images/dockerfile_best-practices/)
- [Multi-stage builds](https://docs.docker.com/build/building/multi-stage/)
- [Docker Compose specification](https://docs.docker.com/compose/compose-file/)
- [Alpine Linux Docker images](https://hub.docker.com/_/alpine)
- [Go official images](https://hub.docker.com/_/golang)

## Reviewers

- ADR Author: Claude (AI Assistant)
- Approved By: v.loginov
- Date: 2026-05-04

## Changelog

- 2026-05-04: Initial ADR documenting Docker containerization decision
