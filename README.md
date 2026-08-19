# Mesh2Mesh
VPN mesh service

## Layout

| Path                  | What it is                                                 |
| --------------------- | ---------------------------------------------------------- |
| `cmd/controlplane`    | HTTP API: tenants, enrollment tokens, peer registration     |
| `cmd/meshd`           | Node CLI: tenants, registration, systemd unit, `mesh0`      |
| `internal/store`      | Postgres persistence and mesh address allocation            |
| `internal/api`        | HTTP handlers                                               |
| `internal/client`     | Typed HTTP client the CLI uses to call the control plane    |
| `migrations`          | SQL schema, applied on first postgres start                 |

# Setup 

## Pre-commit 

```sh
sh setup-precommit.sh
```

Installs a hook that runs `gofmt`, `go vet` and `go build` before each commit.

## Database

```sh
docker compose up -d postgres
```

`migrations/0001_init.sql` is mounted into the container's init directory, so it
runs automatically the first time the `pgdata` volume is created. After editing
the schema, either apply the change by hand with `psql` or start clean:

```sh
docker compose down -v && docker compose up -d postgres
```

## Control plane

```sh
go run ./cmd/controlplane
```

Configuration is environment-only:

| Variable       | Default                                                          |
| -------------- | ---------------------------------------------------------------- |
| `DATABASE_URL` | `postgres://mesh:mesh@localhost:5432/mesh2mesh?sslmode=disable`  |
| `LISTEN_ADDR`  | `:8090`                                                          |

## Node CLI

`cmd/meshd` drives the whole flow from a node, so none of the `curl` calls
below are needed by hand.

```sh
go build -o meshd ./cmd/meshd
```

| Command                | What it does                                              |
| ---------------------- | --------------------------------------------------------- |
| `meshd tenant create`  | `POST /v1/tenants`                                          |
| `meshd tenant token`   | `POST /v1/tenants/{id}/tokens`                              |
| `meshd register`       | `POST /v1/peers/register`, then saves the node state file   |
| `meshd install`        | Writes and starts the systemd unit (runs `meshd up`)        |
| `meshd up`             | Configures `mesh0` from the node state file                 |

Every command takes `--api` (env `MESH2MESH_API`, default
`http://localhost:8090`) and `--state` (env `MESH2MESH_STATE`, default
`/etc/mesh2mesh/peer.json`).

```sh
./meshd tenant create --name acme --cidr 10.203.0.0/16
./meshd tenant token --tenant <tenant-uuid> --ttl 2h --max-uses 3
sudo ./meshd register --token m2m_...        # --name defaults to the hostname
sudo ./meshd up
```

# API

(No auth for now)

### Create a tenant

```sh
curl -sX POST localhost:8090/v1/tenants \
  -H 'content-type: application/json' \
  -d '{"name":"acme","cidr":"10.203.0.0/16"}'
```

```json
{"id":"<tenant-uuid>","name":"acme","cidr":"10.203.0.0/16","created_at":"..."}
```

### Issue an enrollment token

Both fields are optional: `ttl_seconds` defaults to 3600 (max 30 days),
`max_uses` to 1 (max 1000).

```sh
curl -sX POST localhost:8090/v1/tenants/<tenant-uuid>/tokens \
  -H 'content-type: application/json' \
  -d '{"ttl_seconds":3600,"max_uses":1}'
```

```json
{"id":"...","tenant_id":"...","token":"m2m_...","max_uses":1,"expires_at":"..."}
```

The plaintext token is returned **once**. Only its SHA-256 is stored, so it
cannot be read back later.

### Register a peer

The client redeems the token and gets back the address to configure on its mesh
interface.

```sh
curl -sX POST localhost:8090/v1/peers/register \
  -H 'content-type: application/json' \
  -d '{"token":"m2m_...","name":"laptop","public_key":"<wg-pubkey>"}'
```

```json
{
  "peer_id": "...",
  "tenant_id": "...",
  "tenant_name": "acme",
  "name": "laptop",
  "mesh_ip": "10.203.0.2",
  "mesh_cidr": "10.203.0.0/16"
}
```

### Other endpoints

| Method | Path                        | Purpose                              |
| ------ | --------------------------- | ------------------------------------ |
| `GET`  | `/healthz`                  | Liveness + database reachability      |
| `GET`  | `/v1/tenants`               | List tenants                          |
| `GET`  | `/v1/tenants/{id}`          | Fetch one tenant                      |
| `GET`  | `/v1/tenants/{id}/peers`    | List a tenant's registered peers      |
