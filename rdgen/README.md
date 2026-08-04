# RDGen component

RDGen is embedded in the `rustdesk-api` repository. The Django application,
tests, migrations, standalone development files, and build assets are stored
under `rdgen/`; the GitHub Actions files that GitHub must discover are stored
at the repository root under `.github/`.

The production Full S6 image uses this directory directly. It does not clone
or require a separate `rdgen` repository.

## Active build path

The integrated administrator UI currently accepts Windows 64-bit and Windows
32-bit builds. It uses these root workflows and assets:

- `.github/workflows/generator-windows.yml`
- `.github/workflows/generator-windows-x86.yml`
- `.github/workflows/fetch-encrypted-secrets.yml`
- `.github/workflows/bridge.yml`
- `.github/workflows/third-party-RustDeskTempTopMostWindow.yml`
- `.github/actions/decrypt-secrets/`
- `.github/patches/`

Older Android, Linux, macOS, self-hosted Windows, web, cache, and standalone
image workflows are retained in `rdgen/legacy-workflows/` for future porting.
They are intentionally outside `.github/workflows/`, so GitHub does not
register workflows that are not compatible with the current outbound Artifact
flow.

## Build flow

1. The RDGen service encrypts the validated build configuration and uploads it
   as an unreferenced Git blob in the same `rustdesk-api` repository.
2. It dispatches a root generator workflow with the blob SHA and build UUID.
3. GitHub Actions builds the selected RustDesk source and uploads an Artifact
   named `rdgen-<build-uuid>`.
4. The S6 poller downloads and validates the complete EXE/MSI set, then removes
   the remote Actions Artifacts.

The generator host therefore needs outbound GitHub HTTPS access, but no public
callback URL.

## Local development

See [setup.md](setup.md). Production deployments should use the root
[`docs/full-s6-generator.md`](../docs/full-s6-generator.md) guide.
