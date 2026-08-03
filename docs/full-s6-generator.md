# Full S6 image with RustDesk client generator

`Dockerfile_full_s6_generator` builds the generator source stored in this
repository's `rdgen/` directory into RustDesk API Web. RDGen is no longer
cloned from or dispatched through a separate repository. It is a single
authenticated administration interface, not two separate websites. A single
container runs:

- `hbbs` and `hbbr`
- RustDesk API on port `21114`
- the internal rdgen service on port `8000`

Administrators open RustDesk API Web on port `21114` and select
`系统管理 -> 客户端生成器`. The page, build status, saved artifacts, and
downloads all use the existing API Web login. Port `8000` is bound to localhost
only for diagnostics and is not a user-facing frontend.

The container does not compile Windows programs locally. rdgen sends an
encrypted configuration to GitHub and dispatches the generator workflow in
`AllenMGu/rustdesk-api`; GitHub-hosted Windows runners compile the selected
`AllenMGu/rustdesk` source ref. When compilation finishes, the runner uploads
the EXE/MSI to GitHub Actions Artifacts. The S6 poller queries the run, downloads
the files to local persistent storage, verifies the complete output set, and
then deletes all Artifacts belonging to that run.

## 1. Configure the rustdesk-api repository

Add these Actions repository secrets to `AllenMGu/rustdesk-api`:

- `ZIP_PASSWORD`: the same value as `RDGEN_ZIP_PASSWORD`

Optional signing secrets used by the existing workflows can be configured
separately.

Create a fine-grained GitHub token restricted to `AllenMGu/rustdesk-api`. It must be
configured with repository `Actions: Read and write` and
`Contents: Read and write`. The contents permission is required because S6
uploads the encrypted input as an unreferenced Git blob. Put the token only in
the container environment as `RDGEN_GITHUB_TOKEN`; never commit it.

GitHub Actions does not connect back to S6. No `GENURL`, public callback route,
inbound NAT rule, or `RDGEN_UPLOAD_TOKEN` is required by this flow.

## 2. Configure and start the container

```sh
cp .env.full-s6-generator.example .env
# Edit .env before continuing.
chmod 600 .env
docker compose --env-file .env \
  -f docker-compose.full-s6-generator.yaml up -d
```

The relevant variables are:

| Variable | Purpose | Default |
|---|---|---|
| `TZ` | Container timezone | `Asia/Shanghai` |
| `RUSTDESK_HOST` | Hostname or IP reachable by clients, without a scheme or port | required |
| `RUSTDESK_API_PUBLIC_URL` | Full browser-facing HTTP(S) API URL | required |
| `RUSTDESK_API_LANG` | API Web language: `zh-CN` or `en` | `zh-CN` |
| `ENCRYPTED_ONLY` | Whether the RustDesk server accepts only encrypted connections | `0` |
| `MUST_LOGIN` | Whether RustDesk clients must log in before use | `N` |
| `RDGEN_SECRET_KEY` | Unique Django secret; generate with `openssl rand -hex 32` | required |
| `RDGEN_INTERNAL_TOKEN` | Authenticates the API proxy to the internal RDGen service | required |
| `RDGEN_GITHUB_USER` | Owner of this repository | `AllenMGu` |
| `RDGEN_GITHUB_REPOSITORY` | Repository containing API and RDGen | `rustdesk-api` |
| `RDGEN_GITHUB_BRANCH` | Branch containing the dispatched RDGen workflows | `master` |
| `RDGEN_GITHUB_TOKEN` | Dispatches runs and reads/deletes Actions Artifacts | required |
| `RDGEN_ZIP_PASSWORD` | Encrypts build inputs; must match the Actions `ZIP_PASSWORD` secret | required |
| `RDGEN_GITHUB_POLL_INTERVAL` | Polling interval in seconds | `60` |
| `RDGEN_GITHUB_BUILD_TIMEOUT` | Maximum polling lifetime in seconds | `21600` |
| `RDGEN_WORKERS` | Internal generator Gunicorn worker count | `2` |
| `RDGEN_THREADS` | Threads per Gunicorn worker | `4` |
| `RDGEN_DEFAULT_PERMANENT_PASSWORD` | Used only when the form password is empty | empty |
| `RUSTDESK_SOURCE_REPOSITORY` | RustDesk source repository in `owner/repository` form | `AllenMGu/rustdesk` |
| `RUSTDESK_SOURCE_REF` | Fixed source branch, tag, or commit | `master` |

A non-empty `RUSTDESK_SOURCE_REF` fixes the source checkout for every build
and therefore overrides the source ref implied by the page's version selector.
To build the selected official RustDesk tag instead, set
`RUSTDESK_SOURCE_REPOSITORY=rustdesk/rustdesk` and leave
`RUSTDESK_SOURCE_REF` empty.

The compose file uses host networking so RustDesk's published ports do not
depend on Podman bridge forwarding or a dynamically assigned container IP. It
also works with rootful Podman after a compose provider such as
`podman-compose` is installed:

```sh
podman-compose -f docker-compose.full-s6-generator.yaml up -d
```

Its bind mounts use the SELinux private relabel option (`:Z`), allowing the
RustDesk server, API SQLite database, and rdgen to write to their persistent
directories on enforcing Podman hosts.

Because host networking bypasses Compose port publishing, ensure ports
`21114-21119/tcp` and `21116/udp` are allowed by the host firewall. Keep port
`8000/tcp` closed to remote clients; RustDesk API reaches rdgen through
`127.0.0.1:8000` inside the shared network namespace.

The compose example sets `RDGEN_BIND_HOST=127.0.0.1`, so the internal rdgen
service remains available only to processes on this host even though the
container shares the host network namespace. Do not change this to `0.0.0.0`
unless port `8000` is protected separately. Put an HTTPS reverse proxy in
front of RustDesk API on port `21114`. The Go API exposes generator endpoints
only below the authenticated administrator API; there is no public `/rdgen`
callback proxy.

Persistent state is stored below `./data`:

- `server`: RustDesk server keys and database
- `api`: RustDesk API data
- `rdgen/database`: Django SQLite database
- `rdgen/exe`: completed clients returned by GitHub Actions
- `rdgen/png` and `rdgen/temp-zips`: build images and temporary encrypted input

Deployment-specific client defaults are also stored on the host rather than
in the public repository. Create the file before starting the container:

```sh
mkdir -p data/api
cp config/rdgen-client-defaults.example.json \
  data/api/rdgen-client-defaults.json
chmod 600 data/api/rdgen-client-defaults.json
```

Edit `data/api/rdgen-client-defaults.json` and place the desired server
address, API URL, executable name, application name, company name, and other
supported generator fields there. The frontend falls back to the hostname and
origin currently used to open API Web when this file is absent.

Do not put `permanentPassword`, `unlockPin`, `csrfmiddlewaretoken`, or
`sh_secret_field` in this file; the API removes these fields before returning
defaults to the browser. Set the default permanent password only through
`RDGEN_DEFAULT_PERMANENT_PASSWORD` in `.env`. Saved browser configuration
files also omit password and PIN values.

Completed files remain on the S6 host at:

```text
./data/rdgen/exe/<build-uuid>/<generated-file>
```

Open RustDesk API Web and select `系统管理 -> 客户端生成器` to list and
download all saved clients. Use “删除整组” to remove every local EXE/MSI for
one build UUID. Files otherwise remain until an administrator deletes them and
survive container image updates because this path is a bind mount.

The poll interval defaults to 60 seconds. A run is marked `timed_out` after
`RDGEN_GITHUB_BUILD_TIMEOUT` seconds (default `21600`, six hours). Network or
GitHub API failures are recorded and shown in the generator page. If download,
validation, or GitHub deletion fails, the next poll retries safely. GitHub
Artifacts are deleted only after all expected files are present locally.

At startup, the container copies only the server's public
`/data/id_ed25519.pub` into rdgen's readable data directory. If a JSON preset
uses the local server and leaves `RS_PUB_KEY` empty, rdgen inserts this public
key automatically. The private key is never copied.

## 3. Build the image

The release workflow publishes the multi-architecture tag
`full-s6-generator`. A local build needs an API release directory such as
`amd64/release`:

```sh
docker build \
  --build-arg BUILDARCH=amd64 \
  -f Dockerfile_full_s6_generator \
  -t rustdesk-api:full-s6-generator .
```

The image always embeds the `rdgen/` source from the same checked-out API
commit, so an API commit SHA now identifies both components reproducibly.

After the single-repository change is merged, run the `Build` workflow manually.
If Docker Hub credentials are not configured, set `SKIP_DOCKER_HUB=true` and
leave `SKIP_GHCR=false`. The resulting image is
`ghcr.io/allenmgu/rustdesk-api:full-s6-generator`.

## Security notes

- Do not place GitHub tokens, passwords, private keys, or permanent RustDesk
  passwords in the Dockerfile, compose file, or Git repository.
- Generator creation, artifact listing, downloads, and local deletion require
  an authenticated RustDesk API Web administrator.
- Keep `RDGEN_BIND_HOST=127.0.0.1`; port `8000` is an internal service and
  must not be exposed to remote clients.
- Use HTTPS for RustDesk API Web. No generator callback URL is exposed.
- For this integrated image, an empty `RS_PUB_KEY` is filled from the local
  server's `id_ed25519.pub`. Standalone rdgen deployments still need either
  that field or `RUSTDESK_PUBLIC_KEY_FILE`.
- A preset permanent password is embedded in the generated client. Rotate any
  password that has previously been pasted into chat or logs.
