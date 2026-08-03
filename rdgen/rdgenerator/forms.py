from django import forms
from django.core.validators import RegexValidator
from PIL import Image


SAFE_NAME = RegexValidator(
    r"^[A-Za-z0-9][A-Za-z0-9 ._()-]*$",
    "Use only ASCII letters, numbers, spaces, dots, underscores, hyphens, and parentheses.",
)
SAFE_COMPANY = RegexValidator(
    r"^[A-Za-z0-9][A-Za-z0-9 .,_&()-]*$",
    "Company name contains unsupported characters.",
)
SAFE_HOST = RegexValidator(
    r"^[A-Za-z0-9.:\[\]-]+$",
    "Enter a hostname or IP address, optionally followed by a port.",
)
BASE64_PUBLIC_KEY = RegexValidator(
    r"^[A-Za-z0-9+/=]+$",
    "The RustDesk public key must be base64 text.",
)
ANDROID_APP_ID = RegexValidator(
    r"^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$",
    "Enter a valid Android application ID.",
)
WORKFLOW_UNSAFE_URL = RegexValidator(
    r"^[^\x00-\x20\"'`$\\|<>]+$",
    "URL contains characters that are unsafe in the build workflow.",
)


RUSTDESK_VERSIONS = [
    ('master', 'nightly'),
    ('1.4.9', '1.4.9'),
    ('1.4.8', '1.4.8'),
    ('1.4.7', '1.4.7'),
    ('1.4.6', '1.4.6'),
    ('1.4.5', '1.4.5'),
    ('1.4.4', '1.4.4'),
    ('1.4.3', '1.4.3'),
    ('1.4.2', '1.4.2'),
    ('1.4.1', '1.4.1'),
    ('1.4.0', '1.4.0'),
    ('1.3.9', '1.3.9'),
    ('1.3.8', '1.3.8'),
    ('1.3.7', '1.3.7'),
    ('1.3.6', '1.3.6'),
    ('1.3.5', '1.3.5'),
    ('1.3.4', '1.3.4'),
    ('1.3.3', '1.3.3'),
]


class GenerateForm(forms.Form):
    sh_secret_field = forms.CharField(required=False)
    ui_mode = forms.BooleanField(initial=True, required=False)

    #Platform
    platform = forms.ChoiceField(choices=[('windows','Windows 64Bit'),('windows-x86','Windows 32Bit'),('linux','Linux'),('android','Android'),('macos','macOS')], initial='windows')
    version = forms.ChoiceField(choices=RUSTDESK_VERSIONS, initial='1.4.9')
    help_text="'master' is the development version (nightly build) with the latest features but may be less stable"
    delayFix = forms.BooleanField(initial=True, required=False)

    #General
    exename = forms.CharField(label="Name for EXE file", required=True, validators=[SAFE_NAME])
    appname = forms.CharField(label="Custom App Name", required=False, validators=[SAFE_NAME])
    direction = forms.ChoiceField(widget=forms.RadioSelect, choices=[
        ('incoming', 'Incoming Only'),
        ('outgoing', 'Outgoing Only'),
        ('both', 'Bidirectional')
    ], initial='both')
    installation = forms.ChoiceField(label="Disable Installation", choices=[
        ('installationY', 'No, enable installation'),
        ('installationN', 'Yes, DISABLE installation')
    ], initial='installationY')
    settings = forms.ChoiceField(label="Disable Settings", choices=[
        ('settingsY', 'No, enable settings'),
        ('settingsN', 'Yes, DISABLE settings')
    ], initial='settingsY')
    androidappid = forms.CharField(label="Custom Android App ID (replaces 'com.carriez.flutter_hbb')", required=False, validators=[ANDROID_APP_ID])

    #Custom Server
    serverIP = forms.CharField(label="Host", required=False, validators=[SAFE_HOST])
    apiServer = forms.URLField(label="API Server", required=False, max_length=2048, validators=[WORKFLOW_UNSAFE_URL])
    key = forms.CharField(label="Key", required=False, validators=[BASE64_PUBLIC_KEY])
    RS_PUB_KEY = forms.CharField(label="Key (JSON alias)", required=False, validators=[BASE64_PUBLIC_KEY])
    urlLink = forms.URLField(label="Custom URL for links", required=False, max_length=2048, validators=[WORKFLOW_UNSAFE_URL])
    downloadLink = forms.URLField(label="Custom URL for downloading new versions", required=False, max_length=2048, validators=[WORKFLOW_UNSAFE_URL])
    updateLink = forms.URLField(label="Custom update URL", required=False, max_length=2048, validators=[WORKFLOW_UNSAFE_URL])
    compname = forms.CharField(label="Company name", required=False, validators=[SAFE_COMPANY])

    #Visual
    iconfile = forms.FileField(label="Custom App Icon (in .png format)", required=False, widget=forms.FileInput(attrs={'accept': 'image/png'}))
    logofile = forms.FileField(label="Custom App Logo (in .png format)", required=False, widget=forms.FileInput(attrs={'accept': 'image/png'}))
    privacyfile = forms.FileField(label="Custom privacy screen (in .png format)", required=False, widget=forms.FileInput(attrs={'accept': 'image/png'}))
    iconbase64 = forms.CharField(required=False)
    logobase64 = forms.CharField(required=False)
    privacybase64 = forms.CharField(required=False)
    privacy_wallpaper = forms.CharField(required=False)
    theme = forms.ChoiceField(choices=[
        ('light', 'Light'),
        ('dark', 'Dark'),
        ('system', 'Follow System')
    ], initial='system')
    themeDorO = forms.ChoiceField(choices=[('default', 'Default'),('override', 'Override')], initial='default')

    #Security
    passApproveMode = forms.ChoiceField(choices=[('password','Accept sessions via password'),('click','Accept sessions via click'),('password-click','Accepts sessions via both')],initial='password-click')
    permanentPassword = forms.CharField(widget=forms.PasswordInput(), required=False)
    unlockPin = forms.CharField(widget=forms.PasswordInput(), required=False)
    #runasadmin = forms.ChoiceField(choices=[('false','No'),('true','Yes')], initial='false')
    denyLan = forms.BooleanField(initial=False, required=False)
    enableDirectIP = forms.BooleanField(initial=False, required=False)
    #ipWhitelist = forms.BooleanField(initial=False, required=False)
    autoClose = forms.BooleanField(initial=False, required=False)
    remove_preset_password_warning = forms.BooleanField(initial=False, required=False)
    hideSecuritySettings = forms.BooleanField(initial=False, required=False)
    hideNetworkSettings = forms.BooleanField(initial=False, required=False)
    hideServerSettings = forms.BooleanField(initial=False, required=False)
    hideRemotePrinterSettings = forms.BooleanField(initial=False, required=False)
    hideProxySettings = forms.BooleanField(initial=False, required=False)
    hideWebsocketSettings = forms.BooleanField(initial=False, required=False)
    allowHostnameAsId = forms.BooleanField(initial=False, required=False)
    hide_powered_by_me = forms.BooleanField(initial=False, required=False)
    hide_username_on_card = forms.BooleanField(initial=False, required=False)
    hide_account = forms.BooleanField(initial=False, required=False)
    hideTray = forms.BooleanField(initial=False, required=False)
    hidePassword = forms.BooleanField(initial=False, required=False)
    hideService_Start_Stop = forms.BooleanField(initial=False, required=False)

    #Permissions
    permissionsDorO = forms.ChoiceField(choices=[('default', 'Default'),('override', 'Override')], initial='default', required=False)
    permissionsType = forms.ChoiceField(choices=[('custom', 'Custom'),('full', 'Full Access'),('view','Screen share')], initial='custom')
    enableKeyboard =  forms.BooleanField(initial=True, required=False)
    enableClipboard = forms.BooleanField(initial=True, required=False)
    enableFileTransfer = forms.BooleanField(initial=True, required=False)
    enableAudio = forms.BooleanField(initial=True, required=False)
    enableTCP = forms.BooleanField(initial=True, required=False)
    enableRemoteRestart = forms.BooleanField(initial=True, required=False)
    enableRecording = forms.BooleanField(initial=True, required=False)
    enableBlockingInput = forms.BooleanField(initial=True, required=False)
    enableRemoteModi = forms.BooleanField(initial=False, required=False)
    hidecm = forms.BooleanField(initial=False, required=False)
    enablePrinter = forms.BooleanField(initial=True, required=False)
    enableCamera = forms.BooleanField(initial=True, required=False)
    enableTerminal = forms.BooleanField(initial=True, required=False)
    allow_numeric_one_time_password = forms.BooleanField(initial=False, required=False)
    enable_file_copy_paste = forms.BooleanField(initial=False, required=False)

    #Other
    removeWallpaper = forms.BooleanField(initial=True, required=False)
    disable_check_update = forms.BooleanField(initial=False, required=False)
    enable_udp_punch = forms.BooleanField(initial=False, required=False)
    enable_ipv6_punch = forms.BooleanField(initial=False, required=False)
    allowD3dRender = forms.BooleanField(initial=False, required=False)
    use_texture_render = forms.BooleanField(initial=False, required=False)
    pre_elevate_service = forms.BooleanField(initial=False, required=False)
    sync_init_clipboard = forms.BooleanField(initial=False, required=False)
    collapse_toolbar = forms.BooleanField(initial=False, required=False)
    privacy_mode = forms.BooleanField(initial=False, required=False)
    viewOnly = forms.BooleanField(initial=False, required=False)
    image_quality = forms.ChoiceField(
        choices=[
            ('balanced', 'Balanced'),
            ('low', 'Optimize reaction time'),
            ('best', 'Best image quality'),
            ('custom', 'Custom'),
        ],
        initial='balanced',
        required=False,
    )
    custom_fps = forms.IntegerField(initial=30, min_value=5, max_value=120, required=False)
    view_style = forms.CharField(required=False)

    defaultManual = forms.CharField(widget=forms.Textarea, required=False)
    overrideManual = forms.CharField(widget=forms.Textarea, required=False)

    #custom added features
    cycleMonitor = forms.BooleanField(initial=False, required=False)
    xOffline = forms.BooleanField(initial=False, required=False)
    removeNewVersionNotif = forms.BooleanField(initial=False, required=False)
    hide_chat_voice = forms.BooleanField(initial=False, required=False)
    hide_sensitive_ui = forms.BooleanField(initial=False, required=False)
    hideMenuBar = forms.BooleanField(initial=False, required=False)
    hideQuit = forms.BooleanField(initial=False, required=False)
    addcopy = forms.BooleanField(initial=False, required=False)
    applyprivacy = forms.BooleanField(initial=False, required=False)
    passpolicy = forms.BooleanField(initial=False, required=False)
    no_uninstall = forms.BooleanField(initial=False, required=False)
    disable_install = forms.BooleanField(initial=False, required=False)

    def clean_iconfile(self):
        print("checking icon")
        image = self.cleaned_data['iconfile']
        if image:
            try:
                # Open the image using Pillow
                img = Image.open(image)

                # Check if the image is a PNG (optional, but good practice)
                if img.format != 'PNG':
                    raise forms.ValidationError("Only PNG images are allowed.")

                # Get image dimensions
                width, height = img.size

                # Check for square dimensions
                if width != height:
                    raise forms.ValidationError("Custom App Icon dimensions must be square.")
                
                return image
            except OSError:  # Handle cases where the uploaded file is not a valid image
                raise forms.ValidationError("Invalid icon file.")
            except Exception as e: # Catch any other image processing errors
                raise forms.ValidationError(f"Error processing icon: {e}")
