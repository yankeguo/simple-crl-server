# simple-crl-server

A simple HTTP server that generates and serves Certificate Revocation Lists (CRL) based on a list of revoked certificates.

## Features

- Generates CRL from a list of revoked certificates
- Caches CRL for 1 hour to improve performance
- Serves CRL on all HTTP paths on port 8080
- Uses current Unix timestamp as CRL number
- Persists cache to disk for restart resilience
- **Hot-reload support**: Automatically reloads certificates and revocation list on each CRL generation
- **Graceful shutdown**: Properly handles termination signals (SIGINT, SIGTERM)
- **Health check endpoint**: `/healthz` for liveness and readiness probes
- **Kubernetes-ready**: Perfect for deployment with Secret and ConfigMap updates

## Directory Structure

```
.
├── tls/
│   ├── tls.crt       # CA certificate (PEM format)
│   └── tls.key       # CA private key (PEM format)
├── conf/
│   └── list.txt      # List of revoked certificates
└── temp/             # CRL cache directory (auto-created)
```

## Configuration

### Certificate and Key

Place your CA certificate and private key in the `tls/` directory:
- `./tls/tls.crt` - CA certificate in PEM format
- `./tls/tls.key` - CA private key in PEM format (PKCS8, PKCS1, or EC format)

### Revocation List

Create a file at `./conf/list.txt` with the list of revoked certificates.

**Format:** Each line should follow the pattern:
```
[serial_number]:[epoch]:[reason]
```

- `serial_number`: Certificate serial number in hexadecimal (without 0x prefix)
- `epoch`: Unix timestamp when the certificate was revoked
- `reason`: Revocation reason code (integer)

**Revocation Reason Codes:**
- `0`: Unspecified
- `1`: Key Compromise
- `2`: CA Compromise
- `3`: Affiliation Changed
- `4`: Superseded
- `5`: Cessation of Operation
- `6`: Certificate Hold
- `8`: Remove from CRL
- `9`: Privilege Withdrawn
- `10`: AA Compromise

**Example:**
```
1A2B3C4D5E6F:1704067200:1
7890ABCDEF12:1704153600:0
```

Lines starting with `#` are treated as comments and empty lines are ignored.

## Usage

### Build

```bash
go build -o simple-crl-server
```

### Run

```bash
./simple-crl-server
```

The server will start on port 8080 and serve the CRL on all paths.

### Access CRL

```bash
# Download CRL
curl http://localhost:8080/ -o revocation.crl

# Or from any path
curl http://localhost:8080/crl -o revocation.crl
curl http://localhost:8080/revocation.crl -o revocation.crl
```

### Verify CRL

```bash
# Convert DER to PEM and view
openssl crl -inform DER -in revocation.crl -text -noout
```

## Cache Behavior

- CRL is cached for 1 hour
- Cache is stored in `./temp/` directory with filename pattern `crl-[number].der`
- Metadata is stored as `crl-[number].meta`
- On restart, the server attempts to load the most recent valid cache
- New CRL is generated when cache expires or on first request

## Hot-Reload Support

The server reloads the following files on each CRL generation (when cache expires):
- `./tls/tls.crt` - CA certificate
- `./tls/tls.key` - CA private key
- `./conf/list.txt` - Revocation list

This enables seamless updates in Kubernetes environments:
- Update Secret → New certificate/key automatically loaded
- Update ConfigMap → New revocation list automatically loaded
- No pod restart required
- Changes take effect within cache duration (1 hour max)

## Graceful Shutdown

The server implements graceful shutdown to ensure zero-downtime deployments:

**Signals Handled:**
- `SIGINT` (Ctrl+C) - Interrupt signal
- `SIGTERM` - Termination signal (default Kubernetes pod termination)

**Shutdown Process:**
1. Stop accepting new connections
2. Wait for active requests to complete (up to 30 seconds)
3. Clean up resources
4. Exit gracefully

**Example:**
```bash
# Send termination signal
kill -TERM <pid>

# Or press Ctrl+C
```

In Kubernetes, the pod will gracefully shutdown when terminated, ensuring all in-flight requests complete before the container stops.

## Logging

The server logs:
- Startup information
- Certificate and key loading events
- Revocation list loading events
- CRL generation events
- Warnings for invalid entries in `list.txt`
- Cache loading events
- Shutdown signals and graceful termination

## Docker

### Build Docker Image

```bash
docker build -t simple-crl-server:latest .
```

### Run with Docker

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/tls:/app/tls:ro \
  -v $(pwd)/conf:/app/conf:ro \
  -v $(pwd)/temp:/app/temp \
  simple-crl-server:latest
```

## Kubernetes Deployment

### Prerequisites

1. Create a Secret with your CA certificate and key:

```bash
kubectl create secret generic crl-server-tls \
  --from-file=tls.crt=./tls/tls.crt \
  --from-file=tls.key=./tls/tls.key
```

2. Create a ConfigMap with your revocation list:

```bash
kubectl create configmap crl-server-config \
  --from-file=list.txt=./conf/list.txt
```

### Deploy

```bash
kubectl apply -f k8s-example.yaml
```

### Update Configuration (Hot-Reload)

Update the revocation list without restarting pods:

```bash
# Update ConfigMap
kubectl create configmap crl-server-config \
  --from-file=list.txt=./conf/list.txt \
  --dry-run=client -o yaml | kubectl apply -f -

# Wait for Kubernetes to sync the volume (usually a few seconds)
# New CRL will be generated with updated list on next cache expiration (within 1 hour)
```

Update the certificate and key:

```bash
# Update Secret
kubectl create secret generic crl-server-tls \
  --from-file=tls.crt=./tls/tls.crt \
  --from-file=tls.key=./tls/tls.key \
  --dry-run=client -o yaml | kubectl apply -f -

# Wait for Kubernetes to sync the volume
# New certificate will be used on next CRL generation (within 1 hour)
```

### Force Immediate Reload

To force immediate reload without waiting for cache expiration:

```bash
# Delete all pods to clear cache
kubectl rollout restart deployment/crl-server
```

Or manually delete cache by exec into pod:

```bash
kubectl exec -it deployment/crl-server -- rm -rf /app/temp/*
```

## License

See LICENSE file.
