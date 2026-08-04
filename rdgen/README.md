# RDGen component

RDGen is embedded in the `rustdesk-api` repository. The Django application,
tests, migrations, standalone development files, and build assets are stored
under `rdgen/`; the GitHub Actions files that GitHub must discover are stored
at the repository root under `.github/`.

The production Full S6 image uses this directory directly. It does not clone
or require a separate `rdgen` repository.

## Active build path

The integrated administrator UI accepts Windows 64-bit, Windows 32-bit, Linux,
Android, and macOS builds. It uses these root workflows and assets:

- `.github/workflows/generator-windows.yml`
- `.github/workflows/generator-windows-x86.yml`
- `.github/workflows/generator-linux.yml`
- `.github/workflows/generator-android.yml`
- `.github/workflows/generator-macos.yml`
- `.github/workflows/fetch-encrypted-secrets.yml`
- `.github/workflows/bridge.yml`
- `.github/workflows/third-party-RustDeskTempTopMostWindow.yml`
- `.github/actions/decrypt-secrets/`
- `.github/patches/`

Historical self-hosted, callback-upload, cache-maintenance, and standalone
image workflows were removed after the monorepo migration. Only the root
workflows listed above implement the supported outbound Artifact flow.

## Build flow

1. The RDGen service encrypts the validated build configuration and uploads it
   as an unreferenced Git blob in the same `rustdesk-api` repository.
2. It dispatches a root generator workflow with the blob SHA and build UUID.
3. GitHub Actions builds the selected RustDesk source and uploads one or more
   Artifacts named `rdgen-<build-uuid>` or `rdgen-<build-uuid>-<target>`.
4. The S6 poller combines and validates the platform-specific output set, then
   removes the remote Actions Artifacts.

The generator host therefore needs outbound GitHub HTTPS access, but no public
callback URL.

## Local development

See [setup.md](setup.md). Production deployments should use the root
[`docs/full-s6-generator.md`](../docs/full-s6-generator.md) guide.
