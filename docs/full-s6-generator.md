# Full S6 image with RustDesk client generator

`Dockerfile_full_s6_generator` adds the
[AllenMGu/rdgen](https://github.com/AllenMGu/rdgen) web generator to the full
S6 image. A single container runs:

- `hbbs` and `hbbr`
- RustDesk API on port `21114`
- rdgen on port `8000`

The container does not compile Windows programs locally. rdgen sends an
encrypted configuration to GitHub and dispatches the generator workflow in
`AllenMGu/rdgen`; GitHub-hosted Windows runners compile the selected
`AllenMGu/rustdesk` source ref.

## 1. Configure the rdgen repository

Add these Actions repository secrets to `AllenMGu/rdgen`:

- `ZIP_PASSWORD`: the same value as `RDGEN_ZIP_PASSWORD`
- `GENURL`: the same HTTPS URL as `RDGEN_PUBLIC_URL`

Optional signing secrets used by the existing workflows can be configured
separately.

Create a fine-grained GitHub token restricted to `AllenMGu/rdgen`. It must be
able to dispatch Actions workflows and read workflow runs. Put it only in the
container environment as `RDGEN_GITHUB_TOKEN`; never commit it.

## 2. Configure and start the container

```sh
cp .env.full-s6-generator.example .env
# Edit .env before continuing.
docker compose --env-file .env \
  -f docker-compose.full-s6-generator.yaml up -d
```

The compose example binds rdgen to `127.0.0.1:8000`. Put an HTTPS reverse
proxy in front of it and use that public URL for both `RDGEN_PUBLIC_URL` and
the `GENURL` Actions secret. GitHub runners must be able to fetch encrypted
inputs and upload completed installers to this URL.

Persistent state is stored below `./data`:

- `server`: RustDesk server keys and database
- `api`: RustDesk API data
- `rdgen/database`: Django SQLite database
- `rdgen/exe`, `rdgen/png`, and `rdgen/temp-zips`: generated files

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
- Keep the generator behind authentication or a trusted network boundary.
- Use HTTPS for both the RustDesk API and generator callback URL.
- For this integrated image, an empty `RS_PUB_KEY` is filled from the local
  server's `id_ed25519.pub`. Standalone rdgen deployments still need either
  that field or `RUSTDESK_PUBLIC_KEY_FILE`.
- A preset permanent password is embedded in the generated client. Rotate any
  password that has previously been pasted into chat or logs.
