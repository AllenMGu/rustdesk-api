import io
from functools import wraps
from pathlib import Path
import binascii
import logging
import mimetypes
from django.http import FileResponse, HttpResponse, HttpResponseNotAllowed, JsonResponse
from django.shortcuts import get_object_or_404, render
from django.core.files.base import ContentFile
import os
import secrets
import re
import requests
import base64
import json
import shutil
import uuid
import pyzipper
from django.conf import settings as _settings
from django.db.models import Q
from .custom_config import build_custom_config
from .forms import GenerateForm
from .models import GithubRun
from PIL import Image


logger = logging.getLogger(__name__)

PASSTHROUGH_FIELDS = (
    "ui_mode",
    "updateLink",
    "unlockPin",
    "delayFix",
    "cycleMonitor",
    "xOffline",
    "removeNewVersionNotif",
    "hide_chat_voice",
    "hide_sensitive_ui",
    "hideMenuBar",
    "hideQuit",
    "addcopy",
    "applyprivacy",
    "passpolicy",
    "no_uninstall",
    "disable_install",
)

ARTIFACT_EXTENSIONS = {
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
SAFE_ARTIFACT_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._+-]*$")


def _normalize_build_id(value):
    try:
        return str(uuid.UUID(str(value)))
    except (TypeError, ValueError, AttributeError):
        return None


def _safe_artifact_name(value):
    name = str(value or "")
    if (
        not name
        or Path(name).name != name
        or not SAFE_ARTIFACT_NAME.fullmatch(name)
        or Path(name).suffix.lower() not in ARTIFACT_EXTENSIONS
    ):
        return None
    return name


def _artifact_directory(build_id):
    normalized = _normalize_build_id(build_id)
    if not normalized:
        return None
    return Path(_settings.ARTIFACT_ROOT) / normalized


def _artifact_entries(build_id):
    directory = _artifact_directory(build_id)
    if directory is None or not directory.is_dir():
        return []
    entries = []
    for path in directory.iterdir():
        name = _safe_artifact_name(path.name)
        if not name or not path.is_file() or path.is_symlink():
            continue
        stat = path.stat()
        entries.append(
            {
                "name": name,
                "size": stat.st_size,
                "modified": stat.st_mtime,
            }
        )
    return sorted(entries, key=lambda entry: entry["name"].lower())


def _wants_json(request):
    return (
        request.GET.get("format") == "json"
        or "application/json" in request.headers.get("Accept", "")
    )


def _artifact_payload(build_id):
    return [
        {
            **entry,
            "download_url": (
                f"/api/admin/rdgen/download?uuid={build_id}"
                f"&filename={entry['name']}"
            ),
        }
        for entry in _artifact_entries(build_id)
    ]


def _authorized_upload(request):
    expected = _settings.UPLOAD_TOKEN
    scheme, separator, supplied = request.headers.get("Authorization", "").partition(" ")
    return bool(
        expected
        and separator
        and scheme.lower() == "bearer"
        and secrets.compare_digest(expected, supplied)
    )


def _authorized_internal(request):
    expected = _settings.RDGEN_INTERNAL_TOKEN
    supplied = request.headers.get("X-RDGEN-Token", "")
    return bool(
        expected
        and supplied
        and secrets.compare_digest(expected, supplied)
    )


def _require_internal_if_configured(view):
    @wraps(view)
    def wrapped(request, *args, **kwargs):
        if _settings.RDGEN_INTERNAL_TOKEN and not _authorized_internal(request):
            return JsonResponse({"error": "Invalid internal token"}, status=403)
        return view(request, *args, **kwargs)

    return wrapped


def _public_generator_url(request):
    configured_url = _settings.GENURL.strip().rstrip("/")
    if configured_url:
        if "://" not in configured_url:
            configured_url = f"{_settings.PROTOCOL}://{configured_url}"
        return configured_url
    return f"{_settings.PROTOCOL}://{request.get_host()}"


def _generator_form(request):
    is_json = request.content_type == "application/json"
    if not is_json:
        return GenerateForm(request.POST, request.FILES), False

    try:
        payload = json.loads(request.body or b"{}")
    except json.JSONDecodeError:
        return None, True
    if not isinstance(payload, dict):
        return None, True

    # Saved browser configurations use these names for base64 image data.
    for file_field, base64_field in (
        ("iconfile", "iconbase64"),
        ("logofile", "logobase64"),
        ("privacy_wallpaper", "privacybase64"),
    ):
        value = payload.get(file_field)
        if isinstance(value, str) and value.startswith("data:image/"):
            payload[base64_field] = value
    if payload.get("view_style") is False:
        payload["view_style"] = ""
    if isinstance(payload.get("privacy_wallpaper"), dict):
        payload["privacy_wallpaper"] = ""
    return GenerateForm(payload), True


def _workflow_url(platform, selfhosted):
    workflow = {
        "windows": "generator-windows.yml",
        "windows-x86": "generator-windows-x86.yml",
        "linux": "generator-linux.yml",
        "android": "generator-android.yml",
        "macos": "generator-macos.yml",
    }.get(platform, "generator-windows.yml")
    return (
        f"https://api.github.com/repos/{_settings.GHUSER}/{_settings.REPONAME}"
        f"/actions/workflows/{workflow}/dispatches"
    )


def _upload_input_blob(zip_path):
    encoded = base64.b64encode(Path(zip_path).read_bytes()).decode("ascii")
    response = requests.post(
        (
            f"https://api.github.com/repos/{_settings.GHUSER}/"
            f"{_settings.REPONAME}/git/blobs"
        ),
        json={"content": encoded, "encoding": "base64"},
        headers={
            "Accept": "application/vnd.github+json",
            "Authorization": f"Bearer {_settings.GHBEARER}",
            "X-GitHub-Api-Version": "2026-03-10",
        },
        timeout=60,
    )
    response.raise_for_status()
    blob_sha = response.json().get("sha")
    if not blob_sha or not re.fullmatch(r"[0-9a-f]{40,64}", blob_sha):
        raise ValueError("GitHub did not return a valid input blob SHA")
    return blob_sha


def _source_ref(version):
    if _settings.RUSTDESK_REF:
        return _settings.RUSTDESK_REF
    if _settings.RUSTDESK_REPOSITORY != "rustdesk/rustdesk":
        return "master"
    return "master" if version == "master" else f"refs/tags/{version}"


def _server_public_key():
    key_file = _settings.RUSTDESK_PUBLIC_KEY_FILE.strip()
    if not key_file:
        return ""
    try:
        key = Path(key_file).read_text(encoding="utf-8").strip()
        decoded_key = base64.b64decode(key, validate=True)
    except (OSError, ValueError, binascii.Error):
        return ""
    return key if len(decoded_key) == 32 else ""


def _apply_default_permanent_password(cleaned_data):
    if (
        not cleaned_data.get("permanentPassword")
        and _settings.DEFAULT_PERMANENT_PASSWORD
    ):
        cleaned_data["permanentPassword"] = _settings.DEFAULT_PERMANENT_PASSWORD
    return cleaned_data


@_require_internal_if_configured
def generator_view(request):
    if request.method == 'POST':
        form, is_json = _generator_form(request)
        if form is None:
            return JsonResponse({"error": "Request body must be a JSON object"}, status=400)

        if form.is_valid():
            cleaned_data = _apply_default_permanent_password(form.cleaned_data)
            user_secret = cleaned_data['sh_secret_field']
            selfhosted = bool(user_secret) and secrets.compare_digest(
                _settings.SH_SECRET, user_secret
            )
            platform = cleaned_data['platform']
            version = cleaned_data['version']
            server = cleaned_data['serverIP']
            key = cleaned_data['RS_PUB_KEY'] or cleaned_data['key']
            apiServer = cleaned_data['apiServer']
            urlLink = cleaned_data['urlLink']
            downloadLink = cleaned_data['downloadLink']
            updateLink = cleaned_data['updateLink']
            if not server:
                server = 'rs-ny.rustdesk.com' #default rustdesk server
            if not key and cleaned_data['serverIP']:
                key = _server_public_key()
            if not key:
                if cleaned_data['serverIP']:
                    form.add_error(
                        'RS_PUB_KEY',
                        'A RustDesk server public key is required for a custom server.',
                    )
                else:
                    key = 'OeVuKk5nlHiXp+APNn0Y3pC1Iwpwn44JGqrQCsWqmBw='
            if not apiServer:
                apiServer = server+":21114"
            if not urlLink:
                urlLink = "https://rustdesk.com"
            if not downloadLink:
                downloadLink = "https://rustdesk.com/download"
            appname = cleaned_data['appname']
            if not appname:
                appname = "rustdesk"
            filename = cleaned_data['exename']
            compname = cleaned_data['compname']
            if not compname:
                compname = "Purslane Ltd"
            androidappid = cleaned_data['androidappid']
            if not androidappid:
                androidappid = "com.carriez.flutter_hbb"
            compname = compname.replace("&","\\&")

            if all(char.isascii() for char in filename):
                filename = re.sub(r'[^\w\s-]', '_', filename).strip()
                filename = filename.replace(" ","_")
            else:
                filename = "rustdesk"
            if not all(char.isascii() for char in appname):
                appname = "rustdesk"
            if not form.is_valid():
                if is_json:
                    return JsonResponse({"errors": form.errors.get_json_data()}, status=400)
                return render(request, 'generator.html', {'form': form}, status=400)

            myuuid = str(uuid.uuid4())
            full_url = _public_generator_url(request)
            try:
                iconfile = cleaned_data.get('iconfile')
                if not iconfile:
                    iconfile = cleaned_data.get('iconbase64')
                iconlink_url, iconlink_uuid, iconlink_file = save_png(iconfile,myuuid,full_url,"icon.png")
            except:
                print("failed to get icon, using default")
                iconlink_url = "false"
                iconlink_uuid = "false"
                iconlink_file = "false"
            try:
                logofile = cleaned_data.get('logofile')
                if not logofile:
                    logofile = cleaned_data.get('logobase64')
                logolink_url, logolink_uuid, logolink_file = save_png(logofile,myuuid,full_url,"logo.png")
            except:
                print("failed to get logo")
                logolink_url = "false"
                logolink_uuid = "false"
                logolink_file = "false"
            try:
                privacyfile = cleaned_data.get('privacyfile')
                if not privacyfile:
                    privacyfile = cleaned_data.get('privacybase64')
                privacylink_url, privacylink_uuid, privacylink_file = save_png(privacyfile,myuuid,full_url,"privacy.png")
            except:
                print("failed to get logo")
                privacylink_url = "false"
                privacylink_uuid = "false"
                privacylink_file = "false"

            try:
                decodedCustom = build_custom_config(cleaned_data)
            except ValueError as exc:
                if is_json:
                    return JsonResponse({"error": str(exc)}, status=400)
                form.add_error(None, str(exc))
                return render(request, 'generator.html', {'form': form}, status=400)

            decodedCustomJson = json.dumps(decodedCustom, ensure_ascii=True)
            string_bytes = decodedCustomJson.encode("ascii")
            base64_bytes = base64.b64encode(string_bytes)
            encodedCustom = base64_bytes.decode("ascii")

            url = _workflow_url(platform, selfhosted)
            inputs_raw = {
                "server":server,
                "key":key,
                "apiServer":apiServer,
                "custom":encodedCustom,
                "uuid":myuuid,
                "iconlink_url":iconlink_url,
                "iconlink_uuid":iconlink_uuid,
                "iconlink_file":iconlink_file,
                "logolink_url":logolink_url,
                "logolink_uuid":logolink_uuid,
                "logolink_file":logolink_file,
                "privacylink_url":privacylink_url,
                "privacylink_uuid":privacylink_uuid,
                "privacylink_file":privacylink_file,
                "appname":appname,
                "genurl":full_url,
                "urlLink":urlLink,
                "downloadLink":downloadLink,
                "updateLink":updateLink,
                "rdgen":'true',
                "compname": compname,
                "androidappid":androidappid,
                "filename":filename,
                "source_repository": _settings.RUSTDESK_REPOSITORY,
                "source_ref": _source_ref(version),
            }
            for field in PASSTHROUGH_FIELDS:
                value = cleaned_data.get(field)
                if isinstance(value, bool):
                    value = "true" if value else "false"
                inputs_raw[field] = "" if value is None else str(value)

            temporary_directory = Path("temp_zips")
            temp_json_path = temporary_directory / f"data_{uuid.uuid4()}.json"
            zip_filename = f"secrets_{myuuid}.zip"
            zip_path = temporary_directory / zip_filename
            temporary_directory.mkdir(parents=True, exist_ok=True)

            try:
                with open(temp_json_path, "w") as f:
                    json.dump(inputs_raw, f)

                with pyzipper.AESZipFile(
                    zip_path,
                    'w',
                    compression=pyzipper.ZIP_LZMA,
                    encryption=pyzipper.WZ_AES,
                ) as zf:
                    zf.setpassword(_settings.ZIP_PASSWORD.encode())
                    zf.write(temp_json_path, arcname="secrets.json")
                    asset_directory = Path("png") / myuuid
                    for asset_name in ("icon.png", "logo.png", "privacy.png"):
                        asset_path = asset_directory / asset_name
                        if asset_path.is_file():
                            zf.write(asset_path, arcname=f"assets/{asset_name}")
            except Exception:
                logger.exception("Failed to prepare encrypted build inputs")
                Path(zip_path).unlink(missing_ok=True)
                return JsonResponse(
                    {"error": "Failed to prepare encrypted build inputs"},
                    status=500,
                )
            finally:
                Path(temp_json_path).unlink(missing_ok=True)

            try:
                input_blob_sha = _upload_input_blob(zip_path)
            except (OSError, ValueError, requests.RequestException) as exc:
                Path(zip_path).unlink(missing_ok=True)
                return JsonResponse(
                    {"error": f"Failed to upload encrypted build inputs: {exc}"},
                    status=502,
                )

            data = {
                "ref":_settings.GHBRANCH,
                "inputs":{
                    "version":version,
                    "input_blob_sha": input_blob_sha,
                    "build_uuid": myuuid,
                    "source_repository": _settings.RUSTDESK_REPOSITORY,
                    "source_ref": _source_ref(version),
                },
                "return_run_details": True
            } 
            #print(data)
            headers = {
                'Accept':  'application/vnd.github+json',
                'Content-Type': 'application/json',
                'Authorization': 'Bearer '+_settings.GHBEARER,
                'X-GitHub-Api-Version': '2026-03-10'
            }
            new_github_run = GithubRun(
                uuid=myuuid,
                status="Starting generator...please wait",
                platform=platform,
                filename=filename,
            )
            try:
                response = requests.post(url, json=data, headers=headers)
                if response.status_code == 200:
                    github_data = response.json()
                    new_github_run.github_run_id = github_data.get('workflow_run_id')
                    if not new_github_run.github_run_id:
                        Path(zip_path).unlink(missing_ok=True)
                        return JsonResponse(
                            {"error": "GitHub did not return a workflow run ID"},
                            status=502,
                        )
                    new_github_run.status = "in_progress"
                    new_github_run.save()
                    Path(zip_path).unlink(missing_ok=True)

                    if is_json:
                        return JsonResponse(
                            {
                                "uuid": myuuid,
                                "status": new_github_run.status,
                                "workflow_run_id": new_github_run.github_run_id,
                                "log_url": github_data.get('html_url'),
                                "artifacts_url": f"{full_url}/artifacts?build={myuuid}",
                            },
                            status=202,
                        )
                    return render(request, 'waiting.html', {'filename':filename, 'uuid':myuuid, 'status':"Starting generator...please wait", 'platform':platform, 'log_url': github_data.get('html_url')})
                else:
                    Path(zip_path).unlink(missing_ok=True)
                    return JsonResponse(
                        {
                            "error": "GitHub rejected the start request",
                            "github_status": response.status_code,
                        },
                        status=502,
                    )
            except Exception as e:
                Path(zip_path).unlink(missing_ok=True)
                return JsonResponse({"error": f"Connection error: {str(e)}"}, status=500)
        elif is_json:
            return JsonResponse({"errors": form.errors.get_json_data()}, status=400)
    else:
        form = GenerateForm()
    return render(request, 'generator.html', {'form': form})

@_require_internal_if_configured
def check_for_file(request):
    filename = request.GET.get('filename')
    uuid = request.GET.get('uuid')
    platform = request.GET.get('platform')
    gh_run = get_object_or_404(GithubRun, uuid=uuid)
    github_log_url = f"https://github.com/{_settings.GHUSER}/{_settings.REPONAME}/actions/runs/{gh_run.github_run_id}"

    if _wants_json(request):
        return JsonResponse(
            {
                "uuid": str(gh_run.uuid),
                "status": gh_run.status,
                "log_url": github_log_url,
                "poll_failures": gh_run.poll_failures,
                "last_error": gh_run.last_error,
                "artifacts": (
                    _artifact_payload(str(gh_run.uuid))
                    if gh_run.status == "success"
                    else []
                ),
            }
        )

    if gh_run.status == "success":
        return render(request, 'generated.html', {
            'filename': filename,
            'uuid': uuid,
            'platform': platform,
            'artifacts': _artifact_entries(uuid),
        })
        
    elif gh_run.status in [
        'failure',
        'cancelled',
        'timed_out',
        'skipped',
        'action_required',
        'neutral',
        'stale',
    ]:
        return render(request, 'failure.html', {
            'log_url': github_log_url, 
            'filename': filename, 
            'uuid': uuid, 
            'platform': platform,
            'status': gh_run.status
        })
        
    else:
        return render(request, 'waiting.html', {
            'filename': filename, 
            'uuid': uuid, 
            'status': gh_run.status, 
            'platform': platform, 
            'log_url': github_log_url
        })

@_require_internal_if_configured
def download(request):
    build_id = _normalize_build_id(request.GET.get('uuid'))
    filename = _safe_artifact_name(request.GET.get('filename'))
    if not build_id or not filename:
        return JsonResponse({"error": "Invalid build ID or filename"}, status=400)

    directory = _artifact_directory(build_id)
    file_path = directory / filename
    if not file_path.is_file() or file_path.is_symlink():
        return JsonResponse({"error": "Artifact not found"}, status=404)

    content_type = mimetypes.guess_type(filename)[0] or "application/octet-stream"
    return FileResponse(
        file_path.open("rb"),
        as_attachment=True,
        filename=filename,
        content_type=content_type,
    )


@_require_internal_if_configured
def artifacts(request):
    requested_build = request.GET.get("build")
    if not requested_build and not (
        _authorized_internal(request) or _authorized_upload(request)
    ):
        return JsonResponse({"error": "Authentication required"}, status=401)
    if requested_build:
        build_ids = [_normalize_build_id(requested_build)]
        if not build_ids[0]:
            return JsonResponse({"error": "Invalid build ID"}, status=400)
    else:
        root = Path(_settings.ARTIFACT_ROOT)
        if not root.is_dir():
            build_ids = []
        else:
            build_ids = [
                path.name
                for path in root.iterdir()
                if path.is_dir() and _normalize_build_id(path.name)
            ]

    builds = []
    for build_id in build_ids:
        entries = _artifact_entries(build_id)
        if not entries:
            continue
        builds.append(
            {
                "uuid": build_id,
                "artifacts": _artifact_payload(build_id),
                "modified": max(entry["modified"] for entry in entries),
            }
        )
    builds.sort(key=lambda build: build["modified"], reverse=True)
    if _wants_json(request):
        return JsonResponse({"builds": builds})
    return render(request, "artifacts.html", {"builds": builds})


def delete_artifact_build(request):
    if request.method != "DELETE":
        return HttpResponseNotAllowed(["DELETE"])
    if not (_authorized_internal(request) or _authorized_upload(request)):
        return JsonResponse({"error": "Authentication required"}, status=401)
    build_id = _normalize_build_id(request.GET.get("uuid"))
    if not build_id:
        return JsonResponse({"error": "Invalid build ID"}, status=400)
    directory = _artifact_directory(build_id)
    if not directory.is_dir() or directory.is_symlink():
        return JsonResponse({"error": "Artifact not found"}, status=404)
    shutil.rmtree(directory)
    return HttpResponse(status=204)


@_require_internal_if_configured
def get_png(request):
    build_id = _normalize_build_id(request.GET.get("uuid"))
    filename = request.GET.get("filename")
    if not build_id or filename not in {"icon.png", "logo.png", "privacy.png"}:
        return JsonResponse({"error": "Invalid image path"}, status=400)
    file_path = Path("png") / build_id / filename
    if not file_path.is_file() or file_path.is_symlink():
        return JsonResponse({"error": "Image not found"}, status=404)
    return FileResponse(
        file_path.open("rb"),
        filename=filename,
        content_type="image/png",
    )

def create_github_run(myuuid):
    new_github_run = GithubRun(
        uuid=myuuid,
        status="Starting generator...please wait"
    )
    new_github_run.save()

def update_github_run(request):
    if request.method != "POST":
        return HttpResponseNotAllowed(["POST"])
    if not _authorized_upload(request):
        return JsonResponse({"error": "Invalid upload token"}, status=401)
    try:
        data = json.loads(request.body)
    except json.JSONDecodeError:
        return JsonResponse({"error": "Invalid JSON"}, status=400)
    myuuid = _normalize_build_id(data.get("uuid"))
    mystatus = str(data.get("status") or "")
    if not myuuid or not mystatus or len(mystatus) > 100:
        return JsonResponse({"error": "Invalid build status"}, status=400)
    GithubRun.objects.filter(Q(uuid=myuuid)).update(status=mystatus)
    return HttpResponse('')

def resize_and_encode_icon(imagefile):
    maxWidth = 200
    try:
        with io.BytesIO() as image_buffer:
            for chunk in imagefile.chunks():
                image_buffer.write(chunk)
            image_buffer.seek(0)

            img = Image.open(image_buffer)
            imgcopy = img.copy()
    except (IOError, OSError):
        raise ValueError("Uploaded file is not a valid image format.")

    # Check if resizing is necessary
    if img.size[0] <= maxWidth:
        with io.BytesIO() as image_buffer:
            imgcopy.save(image_buffer, format=imagefile.content_type.split('/')[1])
            image_buffer.seek(0)
            return_image = ContentFile(image_buffer.read(), name=imagefile.name)
        return base64.b64encode(return_image.read())

    # Calculate resized height based on aspect ratio
    wpercent = (maxWidth / float(img.size[0]))
    hsize = int((float(img.size[1]) * float(wpercent)))

    # Resize the image while maintaining aspect ratio using LANCZOS resampling
    imgcopy = imgcopy.resize((maxWidth, hsize), Image.Resampling.LANCZOS)

    with io.BytesIO() as resized_image_buffer:
        imgcopy.save(resized_image_buffer, format=imagefile.content_type.split('/')[1])
        resized_image_buffer.seek(0)

        resized_imagefile = ContentFile(resized_image_buffer.read(), name=imagefile.name)

    # Return the Base64 encoded representation of the resized image
    resized64 = base64.b64encode(resized_imagefile.read())
    #print(resized64)
    return resized64
 
#the following is used when accessed from an external source, like the rustdesk api server
def startgh(request):
    #print(request)
    data_ = json.loads(request.body)
    ####from here run the github action, we need user, repo, access token.
    url = 'https://api.github.com/repos/'+_settings.GHUSER+'/'+_settings.REPONAME+'/actions/workflows/generator-'+data_.get('platform')+'.yml/dispatches'  
    data = {
        "ref": _settings.GHBRANCH,
        "inputs":{
            "server":data_.get('server'),
            "key":data_.get('key'),
            "apiServer":data_.get('apiServer'),
            "custom":data_.get('custom'),
            "uuid":data_.get('uuid'),
            "iconlink":data_.get('iconlink'),
            "logolink":data_.get('logolink'),
            "appname":data_.get('appname'),
            "extras":data_.get('extras'),
            "filename":data_.get('filename')
        }
    } 
    headers = {
        'Accept':  'application/vnd.github+json',
        'Content-Type': 'application/json',
        'Authorization': 'Bearer '+_settings.GHBEARER,
        'X-GitHub-Api-Version': '2026-03-10'
    }
    response = requests.post(url, json=data, headers=headers)
    print(response)
    return HttpResponse(status=204)

def save_png(file, uuid, domain, name):
    file_save_path = "png/%s/%s" % (uuid, name)
    Path("png/%s" % uuid).mkdir(parents=True, exist_ok=True)

    if isinstance(file, str):  # Check if it's a base64 string
        try:
            header, encoded = file.split(';base64,')
            decoded_img = base64.b64decode(encoded)
            file = ContentFile(decoded_img, name=name) # Create a file-like object
        except ValueError:
            print("Invalid base64 data")
            return None  # Or handle the error as you see fit
        except Exception as e:  # Catch general exceptions during decoding
            print(f"Error decoding base64: {e}")
            return None
        
    with open(file_save_path, "wb+") as f:
        for chunk in file.chunks():
            f.write(chunk)
    # imageJson = {}
    # imageJson['url'] = domain
    # imageJson['uuid'] = uuid
    # imageJson['file'] = name
    #return "%s/%s" % (domain, file_save_path)
    return domain, uuid, name

def save_custom_client(request):
    if request.method != "POST":
        return HttpResponseNotAllowed(["POST"])
    if not _authorized_upload(request):
        return JsonResponse({"error": "Invalid upload token"}, status=401)

    uploaded_file = request.FILES.get("file")
    build_id = _normalize_build_id(request.POST.get("uuid"))
    filename = _safe_artifact_name(uploaded_file.name if uploaded_file else None)
    if not uploaded_file or not build_id or not filename:
        return JsonResponse(
            {"error": "A valid build UUID and supported artifact file are required"},
            status=400,
        )

    directory = _artifact_directory(build_id)
    directory.mkdir(parents=True, exist_ok=True)
    destination = directory / filename
    temporary = directory / f".{filename}.{secrets.token_hex(8)}.part"
    try:
        with temporary.open("wb") as target:
            for chunk in uploaded_file.chunks():
                target.write(chunk)
        os.replace(temporary, destination)
    finally:
        temporary.unlink(missing_ok=True)

    return JsonResponse(
        {
            "status": "saved",
            "uuid": build_id,
            "filename": filename,
            "size": destination.stat().st_size,
            "download_url": f"/download?uuid={build_id}&filename={filename}",
        },
        status=201,
    )

def cleanup_secrets(request):
    if request.method != "POST":
        return HttpResponseNotAllowed(["POST"])
    if not _authorized_upload(request):
        return JsonResponse({"error": "Invalid upload token"}, status=401)

    # Pass the UUID as a query param or in JSON body
    try:
        data = json.loads(request.body)
    except json.JSONDecodeError:
        return JsonResponse({"error": "Invalid JSON"}, status=400)
    my_uuid = _normalize_build_id(data.get('uuid'))
    
    if not my_uuid:
        return HttpResponse("Missing UUID", status=400)

    file_path = Path("temp_zips") / f"secrets_{my_uuid}.zip"
    file_path.unlink(missing_ok=True)

    return HttpResponse("Cleanup successful", status=200)

def get_zip(request):
    filename = request.GET.get('filename', '')
    match = re.fullmatch(
        r"secrets_([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\.zip",
        filename,
    )
    if not match or not _normalize_build_id(match.group(1)):
        return JsonResponse({"error": "Invalid filename"}, status=400)
    file_path = Path("temp_zips") / filename
    if not file_path.is_file():
        return JsonResponse({"error": "File not found"}, status=404)
    return FileResponse(
        file_path.open("rb"),
        as_attachment=True,
        filename=filename,
        content_type="application/zip",
    )
