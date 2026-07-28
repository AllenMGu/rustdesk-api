# Full S6 image with RustDesk client generator

`Dockerfile_full_s6_generator` adds the
[AllenMGu/rdgen](https://github.com/AllenMGu/rdgen) generator to RustDesk API
Web. It is a single authenticated administration interface, not two separate
websites. A single container runs:

- `hbbs` and `hbbr`
- RustDesk API on port `21114`
- the internal rdgen service on port `8000`

Administrators open RustDesk API Web on port `21114` and select
`系统管理 -> 客户端生成器`. The page, build status, saved artifacts, and
downloads all use the existing API Web login. Port `8000` is bound to localhost
only for diagnostics and is not a user-facing frontend.

The container does not compile Windows programs locally. rdgen sends an
encrypted configuration to GitHub and dispatches the generator workflow in
`AllenMGu/rdgen`; GitHub-hosted Windows runners compile the selected
`AllenMGu/rustdesk` source ref. When compilation finishes, the runner uploads
the installer back to rdgen in this S6 container.

## 1. Configure the rdgen repository

Add these Actions repository secrets to `AllenMGu/rdgen`:

- `ZIP_PASSWORD`: the same value as `RDGEN_ZIP_PASSWORD`
- `GENURL`: the same HTTPS callback prefix as `RDGEN_PUBLIC_URL`, including
  the `/rdgen` suffix

Optional signing secrets used by the existing workflows can be configured
separately.

Create a fine-grained GitHub token restricted to `AllenMGu/rdgen`. It must be
able to dispatch Actions workflows and read workflow runs. Put it only in the
container environment as `RDGEN_GITHUB_TOKEN`; never commit it.

`RDGEN_UPLOAD_TOKEN` is a separate long random value used only by GitHub
Actions when it uploads completed clients back to the S6 server. It is carried
inside the encrypted build input and is not a repository secret or GitHub
token.

## 2. Configure and start the container

```sh
cp .env.full-s6-generator.example .env
# Edit .env before continuing.
docker compose --env-file .env \
  -f docker-compose.full-s6-generator.yaml up -d
```

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
front of RustDesk API on port `21114`. Use the
same public site plus `/rdgen` for both `RDGEN_PUBLIC_URL` and the `GENURL`
Actions secret, for example:

```text
API Web:           https://rustdesk.example.com
RDGEN_PUBLIC_URL:  https://rustdesk.example.com/rdgen
```

GitHub runners must be able to reach the `/rdgen` callback prefix to fetch
encrypted inputs and upload completed installers. The Go API exposes only the
specific callback routes needed by the workflows; generator administration
and downloads remain protected by the existing API Web administrator login.

Allow large request bodies at the reverse proxy. For Nginx, the rdgen location
should include settings similar to:

```nginx
client_max_body_size 2g;
proxy_read_timeout 900s;
proxy_send_timeout 900s;
```

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
download all saved clients. The files are not automatically deleted and
survive container image updates because this path is a bind mount.

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
  --build-arg RDGEN_REPOSITORY=https://github.com/AllenMGu/rdgen.git \
  --build-arg RDGEN_REF=master \
  -f Dockerfile_full_s6_generator \
  -t rustdesk-api:full-s6-generator .
```

Use a full rdgen commit SHA for `RDGEN_REF` when a reproducible build is
required.

After both repository changes are merged, run the `Build` workflow manually.
If Docker Hub credentials are not configured, set `SKIP_DOCKER_HUB=true` and
leave `SKIP_GHCR=false`. The resulting image is
`ghcr.io/allenmgu/rustdesk-api:full-s6-generator`.

## Security notes

- Do not place GitHub tokens, passwords, private keys, or permanent RustDesk
  passwords in the Dockerfile, compose file, or Git repository.
- Generator creation, artifact listing, and downloads require an authenticated
  RustDesk API Web administrator.
- Keep `RDGEN_BIND_HOST=127.0.0.1`; port `8000` is an internal service and
  must not be exposed to remote clients.
- Use HTTPS for both the RustDesk API and generator callback URL.
- For this integrated image, an empty `RS_PUB_KEY` is filled from the local
  server's `id_ed25519.pub`. Standalone rdgen deployments still need either
  that field or `RUSTDESK_PUBLIC_KEY_FILE`.
- A preset permanent password is embedded in the generated client. Rotate any
  password that has previously been pasted into chat or logs.
