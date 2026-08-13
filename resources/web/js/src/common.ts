import * as zstd from "zstddec";
import { KeyEvent, controlKeyFromJSON, ControlKey } from "./message";
import { KEY_MAP, LANGS } from "./gen_js_from_hbb";

let decompressor: zstd.ZSTDDecoder;

export async function initZstd() {
  const tmp = new zstd.ZSTDDecoder();
  await tmp.init();
  console.log("zstd ready");
  decompressor = tmp;
}

export async function decompress(compressedArray: Uint8Array) {
  const MAX = 1024 * 1024 * 64;
  const MIN = 1024 * 1024;
  let n = 30 * compressedArray.length;
  if (n > MAX) {
    n = MAX;
  }
  if (n < MIN) {
    n = MIN;
  }
  try {
    if (!decompressor) {
      await initZstd();
    }
    return decompressor.decode(compressedArray, n);
  } catch (e) {
    console.error("decompress failed: " + e);
    return undefined;
  }
}

const LANG = getLang();

const WEBCLIENT_LANGS: any = {
  cn: {
    "Remote Desktop": "远程桌面",
    "Connect to another device using its RustDesk ID.": "使用 RustDesk ID 连接到另一台设备。",
    "Control Remote Desktop": "控制远程桌面",
    "Enter the ID shown on the remote device.": "输入远程设备上显示的 ID。",
    "Remote ID": "远程 ID",
    "Connect": "连接",
    "Recent": "最近连接",
    "Favorites": "收藏",
    "Address Book": "地址簿",
    "Devices": "设备",
    "Search": "搜索",
    "Select": "选择",
    "List view": "列表视图",
    "Grid view": "网格视图",
    "Search ID, alias, device, user or tag": "搜索 ID、别名、设备、用户或标签",
    "Tags": "标签",
    "All": "全部",
    "No tags": "无标签",
    "Settings": "设置",
    "General": "常规",
    "Network": "网络",
    "Display": "显示",
    "Account": "账户",
    "About": "关于",
    "Theme": "主题",
    "Light": "明亮",
    "Dark": "深色",
    "Follow system": "跟随系统",
    "Language": "语言",
    "Other": "其他",
    "Adaptive bitrate": "自适应码率",
    "Use balanced image quality as the connection default.": "使用平衡图像质量作为连接默认值。",
    "ID Server": "ID 服务器",
    "Relay Server": "中继服务器",
    "API Server": "API 服务器",
    "Save": "保存",
    "Server settings saved": "服务器设置已保存",
    "Default display mode": "默认显示方式",
    "Original size": "原始尺寸",
    "Fit window": "适应窗口",
    "Default image quality": "默认图像质量",
    "Best quality": "画质最优先",
    "Balanced": "平衡",
    "Optimize reaction time": "速度最优先",
    "Default codec": "默认编解码",
    "Other default options": "其它默认选项",
    "Show remote cursor": "显示远程光标",
    "Enable audio": "启用音频",
    "Enable clipboard": "启用剪贴板",
    "Not signed in": "未登录",
    "Sign in to sync address books and registered devices.": "登录以同步地址簿和已登记设备。",
    "Address books and devices are synchronized.": "地址簿和设备已同步。",
    "Version": "版本",
    "Corresponding source": "对应源码",
    "Back": "返回",
  },
};

export function translate(locale: string, text: string): string {
  const lang = LANG || locale.substring(locale.length - 2).toLowerCase();
  let en = LANGS.en as any;
  let dict = (LANGS as any)[lang];
  if (!dict) dict = en;
  let res = WEBCLIENT_LANGS[lang]?.[text] || dict[text];
  if (!res && lang != "en") res = en[text];
  return res || text;
}

const zCode = "z".charCodeAt(0);
const aCode = "a".charCodeAt(0);

export function mapKey(name: string, isDesktop: Boolean) {
  const tmp = KEY_MAP[name] || name;
  if (tmp.length == 1) {
    const chr = tmp.charCodeAt(0);
    if (!isDesktop && (chr > zCode || chr < aCode))
      return KeyEvent.fromPartial({ unicode: chr });
    else return KeyEvent.fromPartial({ chr });
  }
  const control_key = controlKeyFromJSON(tmp);
  if (control_key == ControlKey.UNRECOGNIZED) {
    console.error("Unknown control key " + tmp);
  }
  return KeyEvent.fromPartial({ control_key });
}

export async function sleep(ms: number) {
  await new Promise((r) => setTimeout(r, ms));
}

function getLang(): string {
  try {
    const preferred = localStorage.getItem("webclient-language") || "";
    if (preferred) return preferred;
    const queryString = window.location.search;
    const urlParams = new URLSearchParams(queryString);
    return urlParams.get("lang") || "";
  } catch (e) {
    return "";
  }
}
