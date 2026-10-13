# Mesh2Mesh

VPN mesh service: a control plane hands out mesh addresses, nodes tunnel IP
between each other over WireGuard (wireguard-go, embedded in `meshd`).

## Quickstart

```sh
sh setup-precommit.sh            # gofmt/vet/build pre-commit hook, once
docker compose up -d postgres    # migrations/ run on a fresh volume
go run ./cmd/controlplane        # :8090
go build -o meshd ./cmd/meshd

./meshd tenant create --name acme
./meshd tenant token --tenant <tenant-uuid>
sudo ./meshd register --token m2m_...   # --name defaults to the hostname
sudo ./meshd run                        # data plane; `meshd up` for the interface alone
```

A node sees peers that registered after it on its next refresh, so a fresh pair
takes up to 15s to reach each other. Schema changes need a clean volume
(`docker compose down -v`) or `psql` by hand.

## Containers

Both images are distroless (no shell, no package manager) and run as the
unprivileged user 65532 on a read-only root filesystem. The meshd image adds
one binary to that, a static busybox installed as the `ip` it shells out to.

```sh
docker compose up -d --build     # postgres + control plane, both read-only

docker build -f Containerfile.meshd -t meshd .
docker run -d --name meshd --network host \
  --read-only --tmpfs /run:uid=65532,gid=65532,mode=0700 \
  --cap-drop ALL --cap-add NET_ADMIN --device /dev/net/tun \
  -v meshd-state:/etc/mesh2mesh meshd
```

`setup` and `join` take the same flags in place of the default `run`. meshd
gets `NET_ADMIN` from a file capability, so every command needs
`--cap-add NET_ADMIN` and `no-new-privileges` must stay off. The state volume
has to be writable by 65532: a root-owned `/etc/mesh2mesh` bind-mounted from the
host will not work.

## Commands

| Command               | What it does                                              |
| --------------------- | --------------------------------------------------------- |
| `meshd tenant create` | Creates a tenant (`--cidr`, default `10.203.0.0/16`)       |
| `meshd tenant token`  | Mints an enrollment token (`--ttl`, `--max-uses`)          |
| `meshd register`      | Redeems a token, writes the node state file                |
| `meshd install`       | Writes and starts the systemd unit (runs `meshd run`)      |
| `meshd up`            | Configures `mesh0` from the state file, then exits         |
| `meshd run`           | Configures `mesh0` and carries traffic (`--port`, `-v`)    |

All of them take `--api` (`MESH2MESH_API`, default `http://localhost:8090`) and
`--state` (`MESH2MESH_STATE`, default `/etc/mesh2mesh/peer.json`).

The control plane is configured by environment only: `DATABASE_URL`
(`postgres://mesh:mesh@localhost:5432/mesh2mesh?sslmode=disable`), `LISTEN_ADDR`
(`:8090`) and `PROBE_ADDR` (`:51821`, must be reachable from the nodes — probe
replies come back to it).

## API

| Method | Path                            | Purpose                               |
| ------ | ------------------------------- | ------------------------------------- |
| `GET`  | `/healthz`                      | Liveness + database reachability      |
| `POST` | `/v1/tenants`                   | `{name, cidr?}`                       |
| `GET`  | `/v1/tenants`                   | List tenants                          |
| `GET`  | `/v1/tenants/{id}`              | Fetch one tenant                      |
| `POST` | `/v1/tenants/{id}/tokens`       | `{ttl_seconds?, max_uses?}`           |
| `GET`  | `/v1/tenants/{id}/peers`        | Peers with `endpoint`/`direct_reachable`/`nat_endpoint` |
| `POST` | `/v1/peers/register`            | `{token, name, public_key}` → mesh address |
| `POST` | `/v1/peers/{id}/endpoint`       | `{udp_port}` → probed endpoint, `probe_port` |
| `POST` | `/v1/peers/{id}/nat-endpoint`   | `{endpoint}` → NAT mapping for hole punching |

The token plaintext is returned **once** (only its SHA-256 is stored). The
endpoint call takes the public IP from the request's source address, not the
body, and the nat-endpoint call only accepts that same IP.

## Connectivity

If the control plane's probe of a node's UDP port is answered, peers send to it
directly. Otherwise the node asks the prober which address it is seen from,
reports it as `nat_endpoint`, and peers hole punch to that. This does not get
through symmetric NATs.

## Layout

| Path                | What it is                                              |
| ------------------- | ------------------------------------------------------- |
| `cmd/controlplane`  | HTTP API server                                          |
| `cmd/meshd`         | Node CLI and WireGuard data plane                        |
| `internal/api`      | HTTP handlers                                            |
| `internal/store`    | Postgres persistence and address allocation              |
| `internal/client`   | Typed API client the CLI uses                            |
| `internal/wire`     | Probe framing, shares the node socket with WireGuard     |
| `internal/prober`   | Control-plane reachability probe                         |
| `migrations`        | SQL schema                                               |
