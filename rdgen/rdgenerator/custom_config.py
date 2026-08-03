"""Translate generator form fields into RustDesk's custom.txt schema."""

from __future__ import annotations

from collections.abc import Mapping


PERMISSION_SETTINGS = {
    "enableKeyboard": "enable-keyboard",
    "enableClipboard": "enable-clipboard",
    "enableFileTransfer": "enable-file-transfer",
    "enableAudio": "enable-audio",
    "enableTCP": "enable-tunnel",
    "enableRemoteRestart": "enable-remote-restart",
    "enableRecording": "enable-record-session",
    "enableBlockingInput": "enable-block-input",
    "enableRemoteModi": "allow-remote-config-modification",
    "enablePrinter": "enable-remote-printer",
    "enableCamera": "enable-camera",
    "enableTerminal": "enable-terminal",
    "allow_numeric_one_time_password": "allow-numeric-one-time-password",
}

ADVANCED_BOOLEAN_SETTINGS = {
    "enableDirectIP": "direct-server",
    "autoClose": "allow-auto-disconnect",
    "removeWallpaper": "allow-remove-wallpaper",
    "remove_preset_password_warning": "remove-preset-password-warning",
    "hideSecuritySettings": "hide-security-settings",
    "hideNetworkSettings": "hide-network-settings",
    "hideServerSettings": "hide-server-settings",
    "hideRemotePrinterSettings": "hide-remote-printer-settings",
    "hideProxySettings": "hide-proxy-settings",
    "hideWebsocketSettings": "hide-websocket-settings",
    "allowHostnameAsId": "allow-hostname-as-id",
    "hide_powered_by_me": "hide-powered-by-me",
    "hide_username_on_card": "hide-username-on-card",
    "enable_udp_punch": "enable-udp-punch",
    "enable_ipv6_punch": "enable-ipv6-punch",
    "enable_file_copy_paste": "enable-file-copy-paste",
    "hideTray": "hide-tray",
    "hidePassword": "disable-change-permanent-password",
    "hideService_Start_Stop": "hide-stop-service",
    "allowD3dRender": "allow-d3d-render",
    "use_texture_render": "use-texture-render",
    "pre_elevate_service": "pre-elevate-service",
    "sync_init_clipboard": "sync-init-clipboard",
    "collapse_toolbar": "collapse-toolbar",
    "privacy_mode": "privacy-mode",
    "viewOnly": "view-only",
}

# These keys are accepted and transported for compatibility with customized
# RustDesk forks. Upstream RustDesk currently ignores them.
FORK_BOOLEAN_SETTINGS = {
    "hide_chat_voice": "hide-chat-voice",
    "hide_sensitive_ui": "hide-sensitive-ui",
    "hideMenuBar": "hide-menu-bar",
    "hideQuit": "hide-quit",
    "addcopy": "add-copy",
    "applyprivacy": "apply-privacy",
    "passpolicy": "password-policy",
    "no_uninstall": "no-uninstall",
}


def _yn(value: object) -> str:
    return "Y" if bool(value) else "N"


def _manual_settings(value: object) -> dict[str, str]:
    settings: dict[str, str] = {}
    for line_number, line in enumerate(str(value or "").splitlines(), start=1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            raise ValueError(
                f"Advanced setting on line {line_number} must use key=value syntax"
            )
        key, raw_value = line.split("=", 1)
        key = key.strip()
        if not key:
            raise ValueError(f"Advanced setting on line {line_number} has an empty key")
        settings[key] = raw_value.strip()
    return settings


def build_custom_config(data: Mapping[str, object]) -> dict[str, object]:
    """Build the decoded RustDesk custom client configuration."""

    result: dict[str, object] = {
        "default-settings": {},
        "override-settings": {},
    }
    default_settings = result["default-settings"]
    override_settings = result["override-settings"]
    assert isinstance(default_settings, dict)
    assert isinstance(override_settings, dict)

    direction = str(data.get("direction") or "both").lower()
    if direction != "both":
        result["conn-type"] = direction

    if data.get("installation") == "installationN" or data.get("disable_install"):
        result["disable-installation"] = "Y"
    if data.get("settings") == "settingsN":
        result["disable-settings"] = "Y"
    if data.get("hide_account"):
        result["disable-account"] = "Y"

    app_name = str(data.get("appname") or "").strip()
    if app_name and app_name.lower() != "rustdesk":
        result["app-name"] = app_name

    permanent_password = str(data.get("permanentPassword") or "")
    if permanent_password:
        result["password"] = permanent_password

    # Keep the existing generator behavior: permissions are defaults unless
    # the caller explicitly selects override mode.
    target = (
        override_settings
        if data.get("permissionsDorO") == "override"
        else default_settings
    )
    target["access-mode"] = str(data.get("permissionsType") or "custom")
    for field, option in PERMISSION_SETTINGS.items():
        target[option] = _yn(data.get(field))

    target["enable-lan-discovery"] = _yn(not bool(data.get("denyLan")))
    target["approve-mode"] = str(data.get("passApproveMode") or "password-click")
    target["verification-method"] = (
        "use-permanent-password" if data.get("hidecm") else "use-both-passwords"
    )
    target["allow-hide-cm"] = _yn(data.get("hidecm"))

    for field, option in ADVANCED_BOOLEAN_SETTINGS.items():
        target[option] = _yn(data.get(field))

    # The form expresses this setting as "disable", while RustDesk stores
    # the positive enable-check-update option.
    target["enable-check-update"] = _yn(not bool(data.get("disable_check_update")))

    theme = str(data.get("theme") or "system")
    if theme != "system":
        theme_target = (
            override_settings if data.get("themeDorO") == "override" else default_settings
        )
        theme_target["theme"] = theme

    image_quality = str(data.get("image_quality") or "balanced")
    target["image-quality"] = image_quality
    custom_fps = data.get("custom_fps")
    if custom_fps not in (None, ""):
        target["custom-fps"] = str(custom_fps)

    view_style = data.get("view_style")
    if view_style not in (None, "", False):
        target["view-style"] = str(view_style)

    for field, option in FORK_BOOLEAN_SETTINGS.items():
        target[option] = _yn(data.get(field))

    default_settings.update(_manual_settings(data.get("defaultManual")))
    override_settings.update(_manual_settings(data.get("overrideManual")))
    return result
