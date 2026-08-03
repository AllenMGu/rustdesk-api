import json
import os
import re
import secrets
import shutil
import zipfile
from pathlib import Path

import requests
from django.conf import settings


SUPPORTED_OUTPUTS = {
    ".apk",
    ".appimage",
    ".deb",
    ".dmg",
    ".exe",
    ".flatpak",
    ".msi",
    ".rpm",
    ".zst",
}
SAFE_OUTPUT_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._+-]*$")
COMPLETION_MARKER = ".complete.json"
TERMINAL_FAILURES = {
    "action_required",
    "cancelled",
    "failure",
    "neutral",
    "skipped",
    "stale",
    "timed_out",
}


def _headers():
    return {
        "Accept": "application/vnd.github+json",
        "Authorization": f"Bearer {settings.GHBEARER}",
        "X-GitHub-Api-Version": "2026-03-10",
    }


def _api_url(path):
    return (
        f"https://api.github.com/repos/{settings.GHUSER}/"
        f"{settings.REPONAME}{path}"
    )


def _valid_output_name(value):
    name = str(value or "")
    return bool(
        name
        and Path(name).name == name
        and SAFE_OUTPUT_NAME.fullmatch(name)
        and Path(name).suffix.lower() in SUPPORTED_OUTPUTS
    )


def _required_output_names(github_run):
    filename = str(github_run.filename or "").strip()
    if not _valid_output_name(f"{filename}.exe"):
        raise ValueError("Build record contains an invalid output filename")
    required = {
        "windows": {f"{filename}.exe", f"{filename}.msi"},
        "windows-x86": {f"{filename}.exe"},
        "android": {
            f"{filename}-aarch64.apk",
            f"{filename}-armv7.apk",
            f"{filename}-x86_64.apk",
        },
        "linux": {
            f"{filename}-aarch64.deb",
            f"{filename}-x86_64.deb",
        },
        "macos": {
            f"{filename}-aarch64.dmg",
            f"{filename}-x86_64.dmg",
        },
    }.get(github_run.platform)
    if required is None:
        raise ValueError("Build record contains an unsupported platform")
    return required


def _valid_output_for_run(value, github_run):
    name = str(value or "")
    if not _valid_output_name(name):
        return False
    filename = re.escape(str(github_run.filename or "").strip())
    patterns = {
        "windows": rf"{filename}\.(?:exe|msi)",
        "windows-x86": rf"{filename}\.exe",
        "android": rf"{filename}-(?:aarch64|armv7|x86_64)\.apk",
        "linux": (
            rf"{filename}-(?:suse-)?(?:aarch64|x86_64)\."
            rf"(?:AppImage|deb|flatpak|rpm|pkg\.tar\.zst)"
        ),
        "macos": rf"{filename}-(?:aarch64|x86_64)\.dmg",
    }
    pattern = patterns.get(github_run.platform)
    return bool(pattern and re.fullmatch(pattern, name))


def _has_downloaded_outputs(github_run):
    directory = Path(settings.ARTIFACT_ROOT) / str(github_run.uuid)
    marker = directory / COMPLETION_MARKER
    if not marker.is_file() or marker.is_symlink():
        return False
    try:
        payload = json.loads(marker.read_text(encoding="utf-8"))
    except (OSError, ValueError, TypeError):
        return False
    names = payload.get("files")
    if not isinstance(names, list) or not names:
        return False
    saved = set(names)
    if len(saved) != len(names) or not _required_output_names(github_run) <= saved:
        return False
    return all(
        (directory / name).is_file()
        and not (directory / name).is_symlink()
        and (directory / name).stat().st_size > 0
        and _valid_output_for_run(name, github_run)
        for name in names
    )


def _request_json(path):
    response = requests.get(
        _api_url(path),
        headers=_headers(),
        timeout=30,
    )
    response.raise_for_status()
    payload = response.json()
    if not isinstance(payload, dict):
        raise ValueError("GitHub returned an invalid JSON response")
    return payload


def _delete_artifact(artifact_id):
    if not isinstance(artifact_id, int) or artifact_id <= 0:
        raise ValueError("GitHub returned an invalid artifact ID")
    response = requests.delete(
        _api_url(f"/actions/artifacts/{artifact_id}"),
        headers=_headers(),
        timeout=30,
    )
    if response.status_code not in {204, 404}:
        response.raise_for_status()


def _delete_run_artifacts(artifacts):
    for artifact in artifacts:
        if not isinstance(artifact, dict):
            raise ValueError("GitHub returned an invalid artifact record")
        artifact_id = artifact.get("id")
        if artifact_id:
            _delete_artifact(artifact_id)


def _download_artifacts(artifacts, github_run):
    artifact_root = Path(settings.ARTIFACT_ROOT)
    artifact_root.mkdir(parents=True, exist_ok=True)
    destination = artifact_root / str(github_run.uuid)
    token = secrets.token_hex(8)
    staging = artifact_root / f".{github_run.uuid}.{token}.download"
    archive_path = artifact_root / f".{github_run.uuid}.{token}.zip"
    marker_temp = destination / f"{COMPLETION_MARKER}.{token}.tmp"
    staging.mkdir()
    saved = set()
    try:
        for artifact in artifacts:
            archive_url = artifact.get("archive_download_url")
            if not isinstance(archive_url, str) or not archive_url:
                raise ValueError("GitHub artifact has no download URL")
            response = requests.get(
                archive_url,
                headers=_headers(),
                stream=True,
                timeout=(30, 900),
            )
            response.raise_for_status()
            try:
                with archive_path.open("wb") as target:
                    for chunk in response.iter_content(chunk_size=1024 * 1024):
                        if chunk:
                            target.write(chunk)

                with zipfile.ZipFile(archive_path) as archive:
                    for member in archive.infolist():
                        name = Path(member.filename).name
                        if member.is_dir() or not _valid_output_for_run(
                            name, github_run
                        ):
                            continue
                        if name in saved:
                            raise ValueError(
                                f"GitHub artifact contains duplicate output {name}"
                            )
                        output = staging / name
                        with archive.open(member) as source, output.open(
                            "wb"
                        ) as target:
                            while chunk := source.read(1024 * 1024):
                                target.write(chunk)
                        saved.add(name)
            except (zipfile.BadZipFile, EOFError) as exc:
                raise ValueError(
                    "GitHub artifact is not a valid ZIP archive"
                ) from exc
            finally:
                archive_path.unlink(missing_ok=True)

        required = _required_output_names(github_run)
        missing = required - saved
        if missing:
            raise ValueError(
                "GitHub artifact is incomplete; missing " + ", ".join(sorted(missing))
            )
        empty = {name for name in saved if (staging / name).stat().st_size == 0}
        if empty:
            raise ValueError(
                "GitHub artifact contains empty output files: "
                + ", ".join(sorted(empty))
            )

        destination.mkdir(parents=True, exist_ok=True)
        for name in saved:
            os.replace(staging / name, destination / name)
        marker_temp.write_text(
            json.dumps({"files": sorted(saved)}, separators=(",", ":")),
            encoding="utf-8",
        )
        os.replace(marker_temp, destination / COMPLETION_MARKER)
        return sorted(saved)
    finally:
        archive_path.unlink(missing_ok=True)
        marker_temp.unlink(missing_ok=True)
        shutil.rmtree(staging, ignore_errors=True)


def sync_github_run(github_run):
    if not github_run.github_run_id:
        return False
    outputs_downloaded = _has_downloaded_outputs(github_run)

    if not outputs_downloaded:
        run = _request_json(f"/actions/runs/{github_run.github_run_id}")
        if run.get("status") != "completed":
            status = run.get("status") or "in_progress"
            if github_run.status != status:
                github_run.status = status
                github_run.save(update_fields=["status"])
            return False

        conclusion = run.get("conclusion") or "failure"
        if conclusion != "success":
            github_run.status = conclusion
            github_run.save(update_fields=["status"])
            return False

    payload = _request_json(
        f"/actions/runs/{github_run.github_run_id}/artifacts?per_page=100"
    )
    expected_name = f"rdgen-{github_run.uuid}"
    artifacts = payload.get("artifacts", [])
    if not isinstance(artifacts, list):
        raise ValueError("GitHub returned an invalid artifact list")
    if any(not isinstance(item, dict) for item in artifacts):
        raise ValueError("GitHub returned an invalid artifact record")
    output_artifacts = [
        item
        for item in artifacts
        if not item.get("expired")
        and (
            item.get("name") == expected_name
            or str(item.get("name") or "").startswith(f"{expected_name}-")
        )
    ]
    if outputs_downloaded:
        if output_artifacts:
            github_run.status = "deleting_artifact"
            github_run.save(update_fields=["status"])
        _delete_run_artifacts(artifacts)
        github_run.status = "success"
        github_run.save(update_fields=["status"])
        return True

    if not output_artifacts:
        if github_run.status != "artifact_pending":
            github_run.status = "artifact_pending"
            github_run.save(update_fields=["status"])
        return False

    github_run.status = "downloading_artifacts"
    github_run.save(update_fields=["status"])
    _download_artifacts(output_artifacts, github_run)
    github_run.status = "deleting_artifact"
    github_run.save(update_fields=["status"])
    _delete_run_artifacts(artifacts)
    github_run.status = "success"
    github_run.save(update_fields=["status"])
    return True
