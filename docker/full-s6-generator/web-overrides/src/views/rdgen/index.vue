<template>
  <div class="generator-page">
    <el-page-header title="RustDesk API" content="客户端生成器"/>

    <el-alert
      class="notice"
      type="info"
      :closable="false"
      title="生成任务由 GitHub Actions 编译；完成后的客户端会自动保存到本服务器。永久密码留空时使用服务器端预设值。"
    />

    <div class="toolbar">
      <el-button @click="saveConfig">保存配置</el-button>
      <el-button @click="openConfig">加载配置</el-button>
      <input
        ref="configInput"
        type="file"
        accept="application/json,.json"
        hidden
        @change="loadConfig"
      />
      <el-button type="primary" :loading="creating" @click="submitBuild">
        开始编译
      </el-button>
    </div>

    <el-form label-position="top" class="generator-form">
      <el-card shadow="never">
        <template #header>基本信息</template>
        <el-row :gutter="16">
          <el-col :xs="24" :md="8">
            <el-form-item label="目标平台">
              <el-select v-model="form.platform">
                <el-option label="Windows 64 位" value="windows"/>
                <el-option label="Windows 32 位" value="windows-x86"/>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="RustDesk 版本">
              <el-select v-model="form.version">
                <el-option v-for="version in versions" :key="version" :label="version === 'master' ? 'nightly' : version" :value="version"/>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="连接方向">
              <el-select v-model="form.direction">
                <el-option label="双向" value="both"/>
                <el-option label="仅被控" value="incoming"/>
                <el-option label="仅主控" value="outgoing"/>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="EXE 文件名">
              <el-input v-model="form.exename"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="应用名称">
              <el-input v-model="form.appname"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="公司名称">
              <el-input v-model="form.compname"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="安装功能">
              <el-select v-model="form.installation">
                <el-option label="允许安装" value="installationY"/>
                <el-option label="禁止安装" value="installationN"/>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="设置功能">
              <el-select v-model="form.settings">
                <el-option label="允许设置" value="settingsY"/>
                <el-option label="禁止设置" value="settingsN"/>
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
      </el-card>

      <el-card shadow="never">
        <template #header>服务器和链接</template>
        <el-row :gutter="16">
          <el-col :xs="24" :md="12">
            <el-form-item label="ID/中继服务器">
              <el-input v-model="form.serverIP"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item label="API 地址">
              <el-input v-model="form.apiServer"/>
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="服务器公钥">
              <el-input
                v-model="form.RS_PUB_KEY"
                placeholder="留空时自动读取当前 S6 服务器公钥"
              />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="自定义链接">
              <el-input v-model="form.urlLink"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="下载链接">
              <el-input v-model="form.downloadLink"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="更新链接">
              <el-input v-model="form.updateLink"/>
            </el-form-item>
          </el-col>
        </el-row>
      </el-card>

      <el-card shadow="never">
        <template #header>安全和访问</template>
        <el-row :gutter="16">
          <el-col :xs="24" :md="8">
            <el-form-item label="会话批准方式">
              <el-select v-model="form.passApproveMode">
                <el-option label="密码或点击" value="password-click"/>
                <el-option label="仅密码" value="password"/>
                <el-option label="仅点击" value="click"/>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="永久密码">
              <el-input
                v-model="form.permanentPassword"
                type="password"
                show-password
                autocomplete="new-password"
                placeholder="留空使用服务器端预设密码"
              />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="解锁 PIN">
              <el-input v-model="form.unlockPin" type="password" show-password/>
            </el-form-item>
          </el-col>
        </el-row>
        <div class="switch-grid">
          <label v-for="item in securityOptions" :key="item.key">
            <el-switch v-model="form[item.key]"/>
            <span>{{ item.label }}</span>
          </label>
        </div>
      </el-card>

      <el-card shadow="never">
        <template #header>远程权限</template>
        <el-row :gutter="16">
          <el-col :xs="24" :md="12">
            <el-form-item label="权限模板">
              <el-select v-model="form.permissionsType">
                <el-option label="自定义" value="custom"/>
                <el-option label="完全访问" value="full"/>
                <el-option label="仅屏幕共享" value="view"/>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item label="写入位置">
              <el-select v-model="form.permissionsDorO">
                <el-option label="默认设置" value="default"/>
                <el-option label="强制覆盖" value="override"/>
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <div class="switch-grid">
          <label v-for="item in permissionOptions" :key="item.key">
            <el-switch v-model="form[item.key]"/>
            <span>{{ item.label }}</span>
          </label>
        </div>
      </el-card>

      <el-card shadow="never">
        <template #header>界面和高级设置</template>
        <el-row :gutter="16">
          <el-col :xs="24" :md="8">
            <el-form-item label="主题">
              <el-select v-model="form.theme">
                <el-option label="跟随系统" value="system"/>
                <el-option label="浅色" value="light"/>
                <el-option label="深色" value="dark"/>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="主题写入位置">
              <el-select v-model="form.themeDorO">
                <el-option label="默认设置" value="default"/>
                <el-option label="强制覆盖" value="override"/>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="图像质量">
              <el-select v-model="form.image_quality">
                <el-option label="均衡" value="balanced"/>
                <el-option label="低延迟" value="low"/>
                <el-option label="最佳画质" value="best"/>
                <el-option label="自定义" value="custom"/>
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="自定义 FPS">
              <el-input-number v-model="form.custom_fps" :min="5" :max="120"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item label="默认视图样式">
              <el-input
                v-model="form.view_style"
                placeholder="留空使用 RustDesk 默认值"
              />
            </el-form-item>
          </el-col>
        </el-row>
        <div class="switch-grid">
          <label v-for="item in advancedOptions" :key="item.key">
            <el-switch v-model="form[item.key]"/>
            <span>{{ item.label }}</span>
          </label>
        </div>
        <el-row :gutter="16" class="manual-settings">
          <el-col :xs="24" :md="12">
            <el-form-item label="附加默认设置（每行 key=value）">
              <el-input v-model="form.defaultManual" type="textarea" :rows="4"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item label="附加强制设置（每行 key=value）">
              <el-input v-model="form.overrideManual" type="textarea" :rows="4"/>
            </el-form-item>
          </el-col>
        </el-row>
      </el-card>

      <el-card shadow="never">
        <template #header>品牌图片</template>
        <el-row :gutter="16">
          <el-col v-for="item in imageOptions" :key="item.key" :xs="24" :md="8">
            <el-form-item :label="item.label">
              <input type="file" accept="image/png" @change="loadImage($event, item.key)"/>
              <el-image
                v-if="form[item.key]"
                class="preview"
                :src="form[item.key]"
                fit="contain"
              />
            </el-form-item>
          </el-col>
        </el-row>
      </el-card>
    </el-form>

    <el-card v-if="currentBuild.uuid" shadow="never" class="build-card">
      <template #header>当前编译任务</template>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="任务 UUID">{{ currentBuild.uuid }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="statusType">{{ currentBuild.status }}</el-tag>
        </el-descriptions-item>
      </el-descriptions>
      <el-link
        v-if="currentBuild.log_url"
        :href="currentBuild.log_url"
        target="_blank"
        type="primary"
      >
        查看 GitHub Actions 日志
      </el-link>
      <el-alert
        v-if="currentBuild.last_error"
        class="build-error"
        type="error"
        :closable="false"
        :title="currentBuild.last_error"
      />
    </el-card>

    <el-card shadow="never" class="artifacts-card">
      <template #header>
        <div class="card-header">
          <span>服务器上的客户端</span>
          <el-button @click="refreshArtifacts">刷新</el-button>
        </div>
      </template>
      <el-table :data="artifacts" empty-text="暂无已生成客户端">
        <el-table-column prop="uuid" label="任务 UUID" min-width="290"/>
        <el-table-column prop="name" label="文件名" min-width="220"/>
        <el-table-column label="大小" width="120">
          <template #default="{ row }">{{ formatBytes(row.size) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="180">
          <template #default="{ row }">
            <el-button link type="primary" @click="download(row)">下载</el-button>
            <el-button link type="danger" @click="removeBuild(row)">删除整组</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  createBuild,
  deleteArtifactBuild,
  downloadArtifact,
  getArtifacts,
  getBuildStatus,
  getDefaults,
} from '@/api/rdgen'

const versions = ['master', '1.4.9', '1.4.8', '1.4.7', '1.4.6', '1.4.5', '1.4.4', '1.4.3', '1.4.2', '1.4.1', '1.4.0']
const detectedServer = window.location.hostname
const detectedApiServer = window.location.origin

const form = reactive({
  ui_mode: true,
  platform: 'windows',
  version: '1.4.9',
  exename: 'rustdesk',
  appname: 'RustDesk',
  androidappid: '',
  direction: 'both',
  installation: 'installationY',
  settings: 'settingsN',
  serverIP: detectedServer,
  key: '',
  RS_PUB_KEY: '',
  apiServer: detectedApiServer,
  urlLink: '',
  downloadLink: '',
  updateLink: '',
  compname: '',
  passApproveMode: 'password-click',
  permanentPassword: '',
  unlockPin: '',
  denyLan: true,
  enableDirectIP: true,
  autoClose: true,
  hideSecuritySettings: true,
  hideNetworkSettings: true,
  hideServerSettings: true,
  hideRemotePrinterSettings: true,
  remove_preset_password_warning: true,
  iconfile: '',
  logofile: '',
  privacy_wallpaper: '',
  theme: 'system',
  themeDorO: 'default',
  image_quality: 'balanced',
  custom_fps: 30,
  permissionsDorO: 'default',
  permissionsType: 'custom',
  enableKeyboard: true,
  enableClipboard: true,
  enableFileTransfer: true,
  enableTCP: true,
  enableRemoteRestart: true,
  enableRecording: true,
  enableBlockingInput: true,
  enableRemoteModi: true,
  enableCamera: true,
  enableTerminal: true,
  delayFix: true,
  defaultManual: '',
  overrideManual: '',
  allowHostnameAsId: true,
  disable_check_update: true,
  hide_powered_by_me: true,
  enable_udp_punch: true,
  enable_ipv6_punch: true,
  enable_file_copy_paste: true,
  hide_account: false,
  hideProxySettings: false,
  hideWebsocketSettings: false,
  hidecm: false,
  enableAudio: false,
  enablePrinter: false,
  removeWallpaper: false,
  cycleMonitor: false,
  xOffline: false,
  removeNewVersionNotif: false,
  hide_chat_voice: false,
  collapse_toolbar: false,
  privacy_mode: false,
  hide_username_on_card: false,
  view_style: '',
  hide_sensitive_ui: false,
  hideTray: false,
  hidePassword: false,
  hideMenuBar: false,
  hideQuit: false,
  addcopy: false,
  applyprivacy: false,
  passpolicy: false,
  hideService_Start_Stop: false,
  allow_numeric_one_time_password: false,
  no_uninstall: false,
  disable_install: false,
  allowD3dRender: false,
  viewOnly: false,
  use_texture_render: false,
  pre_elevate_service: false,
  sync_init_clipboard: false,
  sh_secret_field: '',
})

const securityOptions = [
  ['denyLan', '禁止局域网发现'],
  ['enableDirectIP', '允许直接 IP 访问'],
  ['autoClose', '允许自动断开'],
  ['remove_preset_password_warning', '移除预设密码警告'],
  ['hideSecuritySettings', '隐藏安全设置'],
  ['hideNetworkSettings', '隐藏网络设置'],
  ['hideServerSettings', '隐藏服务器设置'],
  ['hideRemotePrinterSettings', '隐藏远程打印设置'],
  ['hideProxySettings', '隐藏代理设置'],
  ['hideWebsocketSettings', '隐藏 WebSocket 设置'],
  ['hidePassword', '禁止修改永久密码'],
  ['hideService_Start_Stop', '隐藏服务启停'],
].map(([key, label]) => ({ key, label }))

const permissionOptions = [
  ['enableKeyboard', '键盘鼠标'],
  ['enableClipboard', '剪贴板'],
  ['enableFileTransfer', '文件传输'],
  ['enable_file_copy_paste', '文件复制粘贴'],
  ['enableAudio', '音频'],
  ['enableTCP', 'TCP 隧道'],
  ['enableRemoteRestart', '远程重启'],
  ['enableRecording', '会话录制'],
  ['enableBlockingInput', '阻止本地输入'],
  ['enableRemoteModi', '允许远程修改配置'],
  ['enablePrinter', '远程打印'],
  ['enableCamera', '摄像头'],
  ['enableTerminal', '终端'],
  ['viewOnly', '仅查看'],
].map(([key, label]) => ({ key, label }))

const advancedOptions = [
  ['ui_mode', 'UI 模式'],
  ['delayFix', '延迟修复'],
  ['allowHostnameAsId', '允许主机名作为 ID'],
  ['disable_check_update', '禁用更新检查'],
  ['hide_powered_by_me', '隐藏 Powered By'],
  ['enable_udp_punch', '启用 UDP 打洞'],
  ['enable_ipv6_punch', '启用 IPv6 打洞'],
  ['hide_account', '隐藏账户'],
  ['hidecm', '隐藏连接管理'],
  ['removeWallpaper', '移除桌面壁纸'],
  ['cycleMonitor', '循环切换显示器'],
  ['xOffline', '离线模式'],
  ['hide_chat_voice', '隐藏聊天/语音'],
  ['collapse_toolbar', '折叠工具栏'],
  ['privacy_mode', '隐私模式'],
  ['hide_username_on_card', '隐藏卡片用户名'],
  ['hide_sensitive_ui', '隐藏敏感界面'],
  ['hideTray', '隐藏托盘'],
  ['hideMenuBar', '隐藏菜单栏'],
  ['hideQuit', '隐藏退出'],
  ['addcopy', '启用复制扩展'],
  ['applyprivacy', '应用隐私扩展'],
  ['passpolicy', '启用密码策略'],
  ['allow_numeric_one_time_password', '允许纯数字一次性密码'],
  ['no_uninstall', '禁止卸载'],
  ['disable_install', '禁用安装'],
  ['allowD3dRender', '允许 D3D 渲染'],
  ['use_texture_render', '使用纹理渲染'],
  ['pre_elevate_service', '预提升服务权限'],
  ['sync_init_clipboard', '连接时同步剪贴板'],
].map(([key, label]) => ({ key, label }))

const imageOptions = [
  { key: 'iconfile', label: '应用图标（正方形 PNG）' },
  { key: 'logofile', label: '应用 Logo（PNG）' },
  { key: 'privacy_wallpaper', label: '隐私屏幕图片（PNG）' },
]

const configInput = ref()
const creating = ref(false)
const currentBuild = reactive({ uuid: '', status: '', log_url: '', last_error: '' })
const artifacts = ref([])
let pollTimer

const statusType = computed(() => {
  if (currentBuild.status === 'success') return 'success'
  if (['failure', 'cancelled', 'timed_out', 'skipped', 'action_required', 'neutral', 'stale'].includes(currentBuild.status)) return 'danger'
  return 'warning'
})

function errorMessage(error) {
  const data = error?.response?.data
  if (data?.error) return data.error
  if (data?.errors) return JSON.stringify(data.errors)
  return error?.message || '请求失败'
}

async function submitBuild() {
  if (!form.exename) {
    ElMessage.error('请填写 EXE 文件名')
    return
  }
  creating.value = true
  try {
    const response = await createBuild({ ...form })
    Object.assign(currentBuild, { last_error: '', ...response.data })
    ElMessage.success('编译任务已提交')
    startPolling()
  } catch (error) {
    ElMessage.error(errorMessage(error))
  } finally {
    creating.value = false
  }
}

function startPolling() {
  clearInterval(pollTimer)
  pollBuild()
  pollTimer = setInterval(pollBuild, 15000)
}

async function pollBuild() {
  if (!currentBuild.uuid) return
  try {
    const response = await getBuildStatus({
      uuid: currentBuild.uuid,
      filename: form.exename,
      platform: form.platform,
    })
    Object.assign(currentBuild, response.data)
    if (['success', 'failure', 'cancelled', 'timed_out', 'skipped', 'action_required', 'neutral', 'stale'].includes(currentBuild.status)) {
      clearInterval(pollTimer)
      if (currentBuild.status === 'success') {
        ElMessage.success('客户端编译完成')
        refreshArtifacts()
      } else {
        ElMessage.error(currentBuild.last_error || `编译结束：${currentBuild.status}`)
      }
    }
  } catch (error) {
    clearInterval(pollTimer)
    ElMessage.error(errorMessage(error))
  }
}

async function refreshArtifacts() {
  try {
    const response = await getArtifacts()
    artifacts.value = response.data.builds.flatMap(build =>
      build.artifacts.map(artifact => ({ ...artifact, uuid: build.uuid })),
    )
  } catch (error) {
    ElMessage.error(errorMessage(error))
  }
}

async function loadServerDefaults() {
  try {
    const response = await getDefaults()
    const defaults = response.data.defaults || {}
    for (const key of Object.keys(form)) {
      if (Object.prototype.hasOwnProperty.call(defaults, key)) {
        form[key] = defaults[key]
      }
    }
  } catch (error) {
    ElMessage.error(`服务器默认配置加载失败：${errorMessage(error)}`)
  }
}

async function download(row) {
  try {
    const response = await downloadArtifact({
      uuid: row.uuid,
      filename: row.name,
    })
    const url = URL.createObjectURL(response.data)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = row.name
    anchor.click()
    setTimeout(() => URL.revokeObjectURL(url), 0)
  } catch (error) {
    ElMessage.error(errorMessage(error))
  }
}

async function removeBuild(row) {
  try {
    await ElMessageBox.confirm(
      `确认删除任务 ${row.uuid} 的全部 EXE/MSI 文件？`,
      '删除服务器安装包',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
      },
    )
    await deleteArtifactBuild({ uuid: row.uuid })
    ElMessage.success('服务器安装包已删除')
    await refreshArtifacts()
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error(errorMessage(error))
  }
}

function loadImage(event, field) {
  const file = event.target.files?.[0]
  if (!file) return
  if (file.type !== 'image/png') {
    ElMessage.error('仅支持 PNG 图片')
    event.target.value = ''
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    form[field] = reader.result
  }
  reader.readAsDataURL(file)
}

function saveConfig() {
  const payload = { ...form, permanentPassword: '', unlockPin: '', sh_secret_field: '' }
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = 'rustdesk-client-config.json'
  anchor.click()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

function openConfig() {
  configInput.value?.click()
}

function loadConfig(event) {
  const file = event.target.files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    try {
      const loaded = JSON.parse(reader.result)
      for (const key of Object.keys(form)) {
        if (Object.prototype.hasOwnProperty.call(loaded, key)) {
          form[key] = loaded[key]
        }
      }
      ElMessage.success('配置已加载；密码字段不会从文件中自动恢复')
    } catch {
      ElMessage.error('配置文件不是有效的 JSON')
    }
  }
  reader.readAsText(file)
  event.target.value = ''
}

function formatBytes(value) {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`
}

onMounted(async () => {
  await loadServerDefaults()
  await refreshArtifacts()
})
onBeforeUnmount(() => clearInterval(pollTimer))
</script>

<style scoped>
.generator-page {
  padding: 20px;
}

.notice,
.toolbar,
.generator-form > .el-card,
.build-card,
.artifacts-card {
  margin-top: 16px;
}

.toolbar {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.switch-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 14px 20px;
}

.switch-grid label {
  display: flex;
  align-items: center;
  gap: 9px;
}

.manual-settings {
  margin-top: 18px;
}

.preview {
  display: block;
  width: 100%;
  height: 120px;
  margin-top: 10px;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.build-card .el-link {
  margin-top: 12px;
}

.build-error {
  margin-top: 12px;
}

@media (max-width: 768px) {
  .generator-page {
    padding: 10px;
  }

  .toolbar {
    flex-wrap: wrap;
  }
}
</style>
