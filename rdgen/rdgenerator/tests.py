import json
import io
import zipfile
from datetime import timedelta
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest.mock import Mock, call, patch

from django.core.files.uploadedfile import SimpleUploadedFile
from django.core.management import call_command
from django.test import RequestFactory, SimpleTestCase, TestCase, override_settings
from django.utils import timezone

from .custom_config import build_custom_config
from .forms import GenerateForm
from .github_artifacts import COMPLETION_MARKER, _delete_artifact, sync_github_run
from .models import GithubRun
from .views import (
    _apply_default_permanent_password,
    _generator_form,
    _safe_artifact_name,
    _server_public_key,
    _upload_input_blob,
)


REQUESTED_FIELDS = {
    "ui_mode",
    "platform",
    "version",
    "exename",
    "appname",
    "androidappid",
    "direction",
    "installation",
    "settings",
    "serverIP",
    "RS_PUB_KEY",
    "apiServer",
    "urlLink",
    "downloadLink",
    "updateLink",
    "compname",
    "passApproveMode",
    "permanentPassword",
    "unlockPin",
    "denyLan",
    "enableDirectIP",
    "autoClose",
    "hideSecuritySettings",
    "hideNetworkSettings",
    "hideServerSettings",
    "hideRemotePrinterSettings",
    "remove_preset_password_warning",
    "iconfile",
    "logofile",
    "privacy_wallpaper",
    "theme",
    "themeDorO",
    "image_quality",
    "custom_fps",
    "permissionsType",
    "enableKeyboard",
    "enableClipboard",
    "enableFileTransfer",
    "enableTCP",
    "enableRemoteRestart",
    "enableRecording",
    "enableBlockingInput",
    "enableRemoteModi",
    "enableCamera",
    "enableTerminal",
    "delayFix",
    "defaultManual",
    "overrideManual",
    "allowHostnameAsId",
    "disable_check_update",
    "hide_powered_by_me",
    "enable_udp_punch",
    "enable_ipv6_punch",
    "enable_file_copy_paste",
    "hide_account",
    "hideProxySettings",
    "hideWebsocketSettings",
    "hidecm",
    "enableAudio",
    "enablePrinter",
    "removeWallpaper",
    "cycleMonitor",
    "xOffline",
    "hide_chat_voice",
    "collapse_toolbar",
    "privacy_mode",
    "hide_username_on_card",
    "view_style",
    "hide_sensitive_ui",
    "hideTray",
    "hidePassword",
    "hideMenuBar",
    "hideQuit",
    "addcopy",
    "applyprivacy",
    "passpolicy",
    "hideService_Start_Stop",
    "allow_numeric_one_time_password",
    "no_uninstall",
    "disable_install",
    "allowD3dRender",
    "viewOnly",
    "use_texture_render",
    "pre_elevate_service",
    "sync_init_clipboard",
}


class GenerateFormTests(SimpleTestCase):
    def test_all_requested_json_fields_are_supported(self):
        self.assertEqual(set(), REQUESTED_FIELDS - set(GenerateForm.base_fields))

    def test_1_4_9_json_payload_is_valid(self):
        request = RequestFactory().post(
            "/generator",
            data=json.dumps(
                {
                    "platform": "windows",
                    "version": "1.4.9",
                    "exename": "example",
                    "appname": "ExampleDesk",
                    "direction": "both",
                    "installation": "installationY",
                    "settings": "settingsN",
                    "serverIP": "rustdesk.example.com",
                    "RS_PUB_KEY": "QUJDRA==",
                    "apiServer": "https://rustdesk.example.com",
                    "theme": "system",
                    "themeDorO": "default",
                    "passApproveMode": "password-click",
                    "permissionsType": "custom",
                    "image_quality": "balanced",
                    "custom_fps": "30",
                    "view_style": False,
                    "privacy_wallpaper": {},
                }
            ),
            content_type="application/json",
        )
        form, is_json = _generator_form(request)
        self.assertTrue(is_json)
        self.assertTrue(form.is_valid(), form.errors)
        self.assertEqual("", form.cleaned_data["view_style"])
        self.assertEqual("", form.cleaned_data["privacy_wallpaper"])

    @override_settings(RUSTDESK_PUBLIC_KEY_FILE="/server/id_ed25519.pub")
    @patch("rdgenerator.views.Path.read_text")
    def test_reads_integrated_server_public_key(self, read_text):
        read_text.return_value = "S2tra2tra2tra2tra2tra2tra2tra2tra2tra2tra2s="
        self.assertEqual(read_text.return_value, _server_public_key())

    @override_settings(DEFAULT_PERMANENT_PASSWORD="server-side-example")
    def test_uses_server_default_when_payload_password_is_empty(self):
        cleaned_data = _apply_default_permanent_password(
            {"permanentPassword": ""}
        )
        self.assertEqual(
            "server-side-example",
            cleaned_data["permanentPassword"],
        )

    @override_settings(DEFAULT_PERMANENT_PASSWORD="server-side-example")
    def test_payload_password_overrides_server_default(self):
        cleaned_data = _apply_default_permanent_password(
            {"permanentPassword": "request-example"}
        )
        self.assertEqual("request-example", cleaned_data["permanentPassword"])

    def test_rejects_urls_that_can_inject_workflow_commands(self):
        base = {
            "platform": "windows",
            "version": "1.4.9",
            "exename": "example",
            "appname": "ExampleDesk",
            "direction": "both",
            "installation": "installationY",
            "settings": "settingsN",
            "serverIP": "rustdesk.example.com",
            "RS_PUB_KEY": "QUJDRA==",
            "theme": "system",
            "themeDorO": "default",
            "passApproveMode": "password-click",
            "permissionsType": "custom",
            "image_quality": "balanced",
        }
        for field in ("apiServer", "urlLink", "downloadLink", "updateLink"):
            form = GenerateForm({
                **base,
                field: "https://example.com/$(whoami)",
            })
            self.assertFalse(form.is_valid())
            self.assertIn(field, form.errors)


class CustomConfigTests(SimpleTestCase):
    def test_maps_requested_options_to_rustdesk_schema(self):
        config = build_custom_config(
            {
                "appname": "ExampleRustDesk",
                "direction": "both",
                "installation": "installationY",
                "settings": "settingsN",
                "permanentPassword": "example-only",
                "permissionsType": "custom",
                "enableKeyboard": True,
                "enableClipboard": True,
                "enableFileTransfer": True,
                "enableAudio": False,
                "enableTCP": True,
                "enableRemoteRestart": True,
                "enableRecording": True,
                "enableBlockingInput": True,
                "enableRemoteModi": True,
                "enablePrinter": False,
                "enableCamera": True,
                "enableTerminal": True,
                "denyLan": True,
                "enableDirectIP": True,
                "autoClose": True,
                "hidecm": False,
                "removeWallpaper": False,
                "remove_preset_password_warning": True,
                "hideSecuritySettings": True,
                "hideNetworkSettings": True,
                "hideServerSettings": True,
                "hideRemotePrinterSettings": True,
                "allowHostnameAsId": True,
                "disable_check_update": True,
                "hide_powered_by_me": True,
                "enable_udp_punch": True,
                "enable_ipv6_punch": True,
                "enable_file_copy_paste": True,
                "image_quality": "balanced",
                "custom_fps": 30,
                "allow_numeric_one_time_password": False,
                "defaultManual": "custom-setting=custom-value",
            }
        )

        self.assertEqual("ExampleRustDesk", config["app-name"])
        self.assertEqual("Y", config["disable-settings"])
        self.assertEqual("example-only", config["password"])
        defaults = config["default-settings"]
        self.assertEqual("N", defaults["enable-lan-discovery"])
        self.assertEqual("Y", defaults["direct-server"])
        self.assertEqual("Y", defaults["remove-preset-password-warning"])
        self.assertEqual("Y", defaults["hide-security-settings"])
        self.assertEqual("N", defaults["enable-check-update"])
        self.assertEqual("Y", defaults["enable-udp-punch"])
        self.assertEqual("Y", defaults["enable-file-copy-paste"])
        self.assertEqual("balanced", defaults["image-quality"])
        self.assertEqual("30", defaults["custom-fps"])
        self.assertEqual("custom-value", defaults["custom-setting"])


class ArtifactStorageTests(SimpleTestCase):
    build_id = "6b5d395f-2478-4ca9-8383-34c0057deab8"

    def test_rejects_unauthorized_upload(self):
        with TemporaryDirectory() as artifact_root, override_settings(
            ARTIFACT_ROOT=Path(artifact_root),
            UPLOAD_TOKEN="test-upload-token",
        ):
            response = self.client.post(
                "/save_custom_client",
                {
                    "uuid": self.build_id,
                    "file": SimpleUploadedFile("client.exe", b"binary"),
                },
            )
        self.assertEqual(401, response.status_code)

    def test_uploads_lists_and_streams_saved_artifact(self):
        with TemporaryDirectory() as artifact_root, override_settings(
            ARTIFACT_ROOT=Path(artifact_root),
            UPLOAD_TOKEN="test-upload-token",
        ):
            response = self.client.post(
                "/save_custom_client",
                {
                    "uuid": self.build_id,
                    "file": SimpleUploadedFile("client.exe", b"binary"),
                },
                HTTP_AUTHORIZATION="Bearer test-upload-token",
            )
            self.assertEqual(201, response.status_code, response.content)
            saved = Path(artifact_root) / self.build_id / "client.exe"
            self.assertEqual(b"binary", saved.read_bytes())

            listing = self.client.get(f"/artifacts?build={self.build_id}")
            self.assertContains(listing, "client.exe")

            json_listing = self.client.get(
                f"/artifacts?build={self.build_id}&format=json"
            )
            self.assertEqual(200, json_listing.status_code)
            payload = json_listing.json()
            self.assertEqual("client.exe", payload["builds"][0]["artifacts"][0]["name"])
            self.assertEqual(
                (
                    f"/api/admin/rdgen/download?uuid={self.build_id}"
                    "&filename=client.exe"
                ),
                payload["builds"][0]["artifacts"][0]["download_url"],
            )

            download = self.client.get(
                f"/download?uuid={self.build_id}&filename=client.exe"
            )
            self.assertEqual(200, download.status_code)
            self.assertEqual(b"binary", b"".join(download.streaming_content))

    def test_rejects_unsafe_artifact_names(self):
        self.assertIsNone(_safe_artifact_name("../client.exe"))
        self.assertIsNone(_safe_artifact_name("client.txt"))

    def test_rejects_image_path_traversal(self):
        response = self.client.get(
            "/get_png",
            {
                "uuid": self.build_id,
                "filename": "/proc/self/environ",
            },
        )
        self.assertEqual(400, response.status_code)

    def test_admin_can_delete_a_saved_build_directory(self):
        with TemporaryDirectory() as artifact_root, override_settings(
            ARTIFACT_ROOT=Path(artifact_root),
            RDGEN_INTERNAL_TOKEN="test-internal-token",
        ):
            directory = Path(artifact_root) / self.build_id
            directory.mkdir()
            (directory / "client.exe").write_bytes(b"binary")
            response = self.client.delete(
                f"/delete_artifact_build?uuid={self.build_id}",
                HTTP_X_RDGEN_TOKEN="test-internal-token",
            )
            self.assertEqual(204, response.status_code)
            self.assertFalse(directory.exists())

    def test_rejects_unauthorized_artifact_deletion(self):
        with TemporaryDirectory() as artifact_root, override_settings(
            ARTIFACT_ROOT=Path(artifact_root),
            RDGEN_INTERNAL_TOKEN="test-internal-token",
        ):
            directory = Path(artifact_root) / self.build_id
            directory.mkdir()
            response = self.client.delete(
                f"/delete_artifact_build?uuid={self.build_id}"
            )
            self.assertEqual(401, response.status_code)
            self.assertTrue(directory.exists())

    def test_internal_token_protects_artifact_downloads(self):
        with TemporaryDirectory() as artifact_root, override_settings(
            ARTIFACT_ROOT=Path(artifact_root),
            RDGEN_INTERNAL_TOKEN="test-internal-token",
        ):
            directory = Path(artifact_root) / self.build_id
            directory.mkdir()
            (directory / "client.exe").write_bytes(b"binary")

            denied = self.client.get(
                f"/download?uuid={self.build_id}&filename=client.exe"
            )
            self.assertEqual(403, denied.status_code)

            allowed = self.client.get(
                f"/download?uuid={self.build_id}&filename=client.exe",
                HTTP_X_RDGEN_TOKEN="test-internal-token",
            )
            self.assertEqual(200, allowed.status_code)
            self.assertEqual(b"binary", b"".join(allowed.streaming_content))


class GitHubInputBlobTests(SimpleTestCase):
    @override_settings(
        GHUSER="AllenMGu",
        REPONAME="rustdesk-api",
        GHBEARER="test-token",
    )
    @patch("rdgenerator.views.requests.post")
    def test_uploads_encrypted_input_as_unreferenced_git_blob(self, post):
        response = Mock()
        response.json.return_value = {"sha": "a" * 40}
        response.raise_for_status.return_value = None
        post.return_value = response

        with TemporaryDirectory() as directory:
            path = Path(directory) / "secrets.zip"
            path.write_bytes(b"encrypted")
            self.assertEqual("a" * 40, _upload_input_blob(path))

        request = post.call_args
        self.assertTrue(request.kwargs["json"]["content"])
        self.assertEqual("base64", request.kwargs["json"]["encoding"])
        self.assertNotIn(b"encrypted", str(request.kwargs).encode())


class GitHubPollingCommandTests(TestCase):
    @override_settings(GITHUB_BUILD_TIMEOUT=600, GITHUB_POLL_INTERVAL=10)
    @patch(
        "rdgenerator.management.commands.poll_github_artifacts.sync_github_run"
    )
    def test_marks_stale_build_as_timed_out_without_polling_github(self, sync):
        github_run = GithubRun.objects.create(
            uuid="8ddf9e50-12c5-44fd-8f45-158147beff23",
            status="in_progress",
            github_run_id=123,
            platform="windows",
            filename="client",
        )
        GithubRun.objects.filter(pk=github_run.pk).update(
            created_at=timezone.now() - timedelta(minutes=11)
        )

        call_command("poll_github_artifacts", once=True)

        github_run.refresh_from_db()
        self.assertEqual("timed_out", github_run.status)
        self.assertIn("polling timeout", github_run.last_error)
        sync.assert_not_called()


class GitHubArtifactPollingTests(TestCase):
    build_id = "6b5d395f-2478-4ca9-8383-34c0057deab8"

    @override_settings(
        GHUSER="AllenMGu",
        REPONAME="rustdesk-api",
        GHBEARER="test-token",
    )
    @patch("rdgenerator.github_artifacts.requests.delete")
    def test_deletes_artifact_through_the_github_api(self, delete):
        response = Mock(status_code=204)
        delete.return_value = response

        _delete_artifact(98765)

        delete.assert_called_once_with(
            "https://api.github.com/repos/AllenMGu/rustdesk-api/actions/artifacts/98765",
            headers={
                "Accept": "application/vnd.github+json",
                "Authorization": "Bearer test-token",
                "X-GitHub-Api-Version": "2026-03-10",
            },
            timeout=30,
        )

    @override_settings(
        GHUSER="AllenMGu",
        REPONAME="rustdesk-api",
        GHBEARER="test-token",
    )
    @patch("rdgenerator.github_artifacts._delete_artifact")
    @patch("rdgenerator.github_artifacts.requests.get")
    @patch("rdgenerator.github_artifacts._request_json")
    def test_downloads_matching_run_artifact_to_build_directory(
        self,
        request_json,
        get,
        delete_artifact,
    ):
        archive = io.BytesIO()
        with zipfile.ZipFile(archive, "w") as output:
            output.writestr("ExampleRustDesk.exe", b"exe")
            output.writestr("ExampleRustDesk.msi", b"msi")
            output.writestr("ignored.txt", b"ignored")

        response = Mock()
        response.iter_content.return_value = [archive.getvalue()]
        response.raise_for_status.return_value = None
        get.return_value = response
        request_json.side_effect = [
            {"status": "completed", "conclusion": "success"},
            {
                "artifacts": [
                    {
                        "id": 98763,
                        "name": "bridge-artifact",
                        "expired": False,
                    },
                    {
                        "id": 98764,
                        "name": "topmostwindow-artifacts",
                        "expired": False,
                    },
                    {
                        "id": 98765,
                        "name": f"rdgen-{self.build_id}",
                        "expired": False,
                        "archive_download_url": "https://api.github.test/artifact.zip",
                    }
                ]
            },
        ]

        github_run = GithubRun.objects.create(
            id=1,
            uuid=self.build_id,
            status="in_progress",
            github_run_id=12345,
            platform="windows",
            filename="ExampleRustDesk",
        )
        with TemporaryDirectory() as artifact_root, override_settings(
            ARTIFACT_ROOT=Path(artifact_root)
        ):
            self.assertTrue(sync_github_run(github_run))
            destination = Path(artifact_root) / self.build_id
            self.assertEqual(b"exe", (destination / "ExampleRustDesk.exe").read_bytes())
            self.assertEqual(b"msi", (destination / "ExampleRustDesk.msi").read_bytes())
            self.assertFalse((destination / "ignored.txt").exists())

        github_run.refresh_from_db()
        self.assertEqual("success", github_run.status)
        delete_artifact.assert_has_calls(
            [call(98763), call(98764), call(98765)]
        )

    @override_settings(
        GHUSER="AllenMGu",
        REPONAME="rustdesk-api",
        GHBEARER="test-token",
    )
    @patch("rdgenerator.github_artifacts._delete_artifact")
    @patch("rdgenerator.github_artifacts._request_json")
    def test_retries_cleanup_without_downloading_the_artifact_again(
        self,
        request_json,
        delete_artifact,
    ):
        request_json.return_value = {
            "artifacts": [
                {
                    "id": 98765,
                    "name": f"rdgen-{self.build_id}",
                    "expired": False,
                    "archive_download_url": "https://api.github.test/artifact.zip",
                }
            ]
        }
        github_run = GithubRun.objects.create(
            id=2,
            uuid=self.build_id,
            status="deleting_artifact",
            github_run_id=12345,
            platform="windows-x86",
            filename="ExampleRustDesk",
        )
        with TemporaryDirectory() as artifact_root, override_settings(
            ARTIFACT_ROOT=Path(artifact_root)
        ):
            destination = Path(artifact_root) / self.build_id
            destination.mkdir(parents=True)
            (destination / "ExampleRustDesk.exe").write_bytes(b"exe")
            (destination / COMPLETION_MARKER).write_text(
                json.dumps({"files": ["ExampleRustDesk.exe"]}),
                encoding="utf-8",
            )
            self.assertTrue(sync_github_run(github_run))

        github_run.refresh_from_db()
        self.assertEqual("success", github_run.status)
        delete_artifact.assert_called_once_with(98765)
        request_json.assert_called_once_with(
            f"/actions/runs/{github_run.github_run_id}/artifacts?per_page=100"
        )

    @override_settings(
        GHUSER="AllenMGu",
        REPONAME="rustdesk-api",
        GHBEARER="test-token",
    )
    @patch("rdgenerator.github_artifacts._delete_artifact")
    @patch("rdgenerator.github_artifacts.requests.get")
    @patch("rdgenerator.github_artifacts._request_json")
    def test_does_not_delete_an_incomplete_windows_artifact(
        self,
        request_json,
        get,
        delete_artifact,
    ):
        archive = io.BytesIO()
        with zipfile.ZipFile(archive, "w") as output:
            output.writestr("ExampleRustDesk.exe", b"exe")

        response = Mock()
        response.iter_content.return_value = [archive.getvalue()]
        response.raise_for_status.return_value = None
        get.return_value = response
        request_json.side_effect = [
            {"status": "completed", "conclusion": "success"},
            {
                "artifacts": [
                    {
                        "id": 98765,
                        "name": f"rdgen-{self.build_id}",
                        "expired": False,
                        "archive_download_url": "https://api.github.test/artifact.zip",
                    }
                ]
            },
        ]

        github_run = GithubRun.objects.create(
            id=3,
            uuid=self.build_id,
            status="in_progress",
            github_run_id=12345,
            platform="windows",
            filename="ExampleRustDesk",
        )
        with TemporaryDirectory() as artifact_root, override_settings(
            ARTIFACT_ROOT=Path(artifact_root)
        ):
            with self.assertRaisesRegex(ValueError, "missing ExampleRustDesk.msi"):
                sync_github_run(github_run)
            self.assertFalse(
                (Path(artifact_root) / self.build_id / COMPLETION_MARKER).exists()
            )

        delete_artifact.assert_not_called()

    @override_settings(
        GHUSER="AllenMGu",
        REPONAME="rustdesk-api",
        GHBEARER="test-token",
    )
    @patch("rdgenerator.github_artifacts._delete_artifact")
    @patch("rdgenerator.github_artifacts.requests.get")
    @patch("rdgenerator.github_artifacts._request_json")
    def test_does_not_delete_a_corrupt_artifact(
        self,
        request_json,
        get,
        delete_artifact,
    ):
        response = Mock()
        response.iter_content.return_value = [b"not-a-zip"]
        response.raise_for_status.return_value = None
        get.return_value = response
        request_json.side_effect = [
            {"status": "completed", "conclusion": "success"},
            {
                "artifacts": [
                    {
                        "id": 98765,
                        "name": f"rdgen-{self.build_id}",
                        "expired": False,
                        "archive_download_url": "https://api.github.test/artifact.zip",
                    }
                ]
            },
        ]
        github_run = GithubRun.objects.create(
            uuid=self.build_id,
            status="in_progress",
            github_run_id=12345,
            platform="windows",
            filename="ExampleRustDesk",
        )

        with TemporaryDirectory() as artifact_root, override_settings(
            ARTIFACT_ROOT=Path(artifact_root)
        ):
            with self.assertRaisesRegex(ValueError, "not a valid ZIP archive"):
                sync_github_run(github_run)

        delete_artifact.assert_not_called()

    @override_settings(
        GHUSER="AllenMGu",
        REPONAME="rustdesk-api",
        GHBEARER="test-token",
    )
    @patch("rdgenerator.github_artifacts._delete_artifact")
    @patch("rdgenerator.github_artifacts.requests.get")
    @patch("rdgenerator.github_artifacts._request_json")
    def test_does_not_delete_empty_output_files(
        self,
        request_json,
        get,
        delete_artifact,
    ):
        archive = io.BytesIO()
        with zipfile.ZipFile(archive, "w") as output:
            output.writestr("ExampleRustDesk.exe", b"")
            output.writestr("ExampleRustDesk.msi", b"msi")

        response = Mock()
        response.iter_content.return_value = [archive.getvalue()]
        response.raise_for_status.return_value = None
        get.return_value = response
        request_json.side_effect = [
            {"status": "completed", "conclusion": "success"},
            {
                "artifacts": [
                    {
                        "id": 98765,
                        "name": f"rdgen-{self.build_id}",
                        "expired": False,
                        "archive_download_url": "https://api.github.test/artifact.zip",
                    }
                ]
            },
        ]
        github_run = GithubRun.objects.create(
            uuid=self.build_id,
            status="in_progress",
            github_run_id=12345,
            platform="windows",
            filename="ExampleRustDesk",
        )

        with TemporaryDirectory() as artifact_root, override_settings(
            ARTIFACT_ROOT=Path(artifact_root)
        ):
            with self.assertRaisesRegex(ValueError, "empty output files"):
                sync_github_run(github_run)

        delete_artifact.assert_not_called()
