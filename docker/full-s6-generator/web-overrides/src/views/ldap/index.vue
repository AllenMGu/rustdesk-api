<template>
  <div class="ldap-page" v-loading="loading">
    <el-page-header title="RustDesk API" content="LDAP / Active Directory"/>

    <el-alert
      class="notice"
      type="info"
      :closable="false"
      title="配置保存后会立即应用。Bind 密码不会回显；留空表示保留现有密码。环境变量控制的字段为只读。"
    />
    <el-alert
      v-if="document.locked_fields?.length"
      class="notice"
      type="warning"
      :closable="false"
      title="部分配置由环境变量控制，页面中的对应字段已锁定。"
    />
    <el-alert
      v-if="!document.can_save_password"
      class="notice"
      type="warning"
      :closable="false"
      title="尚未配置 RUSTDESK_API_SETTINGS_KEY。未使用 Bind 密码时仍可保存；如果当前已有或需要设置 Bind 密码，请先配置该密钥并重启容器。"
    />
    <el-alert
      v-if="document.load_error"
      class="notice"
      type="error"
      :closable="false"
      title="已保存的 LDAP 配置无法读取。请恢复原 RUSTDESK_API_SETTINGS_KEY 并重启容器；为避免覆盖，保存和回滚已被后端阻止。"
    />

    <div class="toolbar">
      <el-button @click="applyActiveDirectoryTemplate">应用通用 AD 模板</el-button>
      <el-button :loading="testing" @click="testConfiguration">测试连接</el-button>
      <el-button
        :disabled="!document.can_rollback"
        :loading="rollingBack"
        @click="rollback"
      >
        恢复上一次配置
      </el-button>
      <el-button type="primary" :disabled="!!document.load_error" :loading="saving" @click="save">保存并立即应用</el-button>
    </div>

    <el-form label-position="top" class="ldap-form">
      <el-card shadow="never">
        <template #header>
          <div class="card-title">
            <span>服务状态与连接</span>
            <el-tag :type="form.enable ? 'success' : 'info'">
              {{ form.enable ? '已启用' : '未启用' }}
            </el-tag>
          </div>
        </template>
        <el-row :gutter="16">
          <el-col :xs="24" :md="8">
            <el-form-item>
              <template #label><field-label title="启用 LDAP" field="enable" :document="document"/></template>
              <el-switch v-model="form.enable" :disabled="isLocked('enable')"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="16">
            <el-form-item>
              <template #label><field-label title="LDAP 地址" field="url" :document="document"/></template>
              <el-input
                v-model="form.url"
                :disabled="isLocked('url')"
                placeholder="ldap://dc.example.com:389 或 ldaps://dc.example.com:636"
              />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item>
              <template #label><field-label title="Base DN" field="base_dn" :document="document"/></template>
              <el-input v-model="form.base_dn" :disabled="isLocked('base_dn')" placeholder="DC=example,DC=com"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item>
              <template #label><field-label title="用户 Base DN" field="user.base_dn" :document="document"/></template>
              <el-input v-model="form.user.base_dn" :disabled="isLocked('user.base_dn')" placeholder="留空时使用 Base DN"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item>
              <template #label><field-label title="Bind DN" field="bind_dn" :document="document"/></template>
              <el-input v-model="form.bind_dn" :disabled="isLocked('bind_dn')" autocomplete="off"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item>
              <template #label><field-label title="Bind Password" field="bind_password" :document="document"/></template>
              <el-input
                v-model="bindPassword"
                type="password"
                show-password
                autocomplete="new-password"
                :disabled="isLocked('bind_password')"
                :placeholder="document.password_configured ? '已配置；留空保持不变' : '请输入 Bind 密码'"
              />
              <el-checkbox
                v-if="document.password_configured && !isLocked('bind_password')"
                v-model="clearBindPassword"
                class="clear-password"
              >
                保存时清除现有 Bind 密码
              </el-checkbox>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item>
              <template #label><field-label title="连接超时（秒）" field="connect_timeout_seconds" :document="document"/></template>
              <el-input-number v-model="form.connect_timeout_seconds" :min="1" :max="120" :disabled="isLocked('connect_timeout_seconds')"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item>
              <template #label><field-label title="操作超时（秒）" field="operation_timeout_seconds" :document="document"/></template>
              <el-input-number v-model="form.operation_timeout_seconds" :min="1" :max="120" :disabled="isLocked('operation_timeout_seconds')"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item>
              <template #label><field-label title="同步用户信息" field="user.sync" :document="document"/></template>
              <el-switch v-model="form.user.sync" :disabled="isLocked('user.sync')"/>
            </el-form-item>
          </el-col>
        </el-row>
      </el-card>

      <el-card shadow="never">
        <template #header>用户搜索与属性</template>
        <el-row :gutter="16">
          <el-col :xs="24" :md="12">
            <el-form-item>
              <template #label><field-label title="登录属性" field="user.username" :document="document"/></template>
              <el-input
                v-model="form.user.username"
                :disabled="isLocked('user.username')"
                placeholder="sAMAccountName,userPrincipalName"
              />
              <div class="field-help">多个属性用英文逗号分隔，可同时支持裸用户名和 UPN。</div>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item>
              <template #label><field-label title="用户过滤器" field="user.filter" :document="document"/></template>
              <el-input v-model="form.user.filter" :disabled="isLocked('user.filter')"/>
            </el-form-item>
          </el-col>
          <el-col v-for="field in attributeFields" :key="field.key" :xs="24" :md="6">
            <el-form-item>
              <template #label><field-label :title="field.label" :field="`user.${field.key}`" :document="document"/></template>
              <el-input v-model="form.user[field.key]" :disabled="isLocked(`user.${field.key}`)"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item>
              <template #label><field-label title="账户启用属性" field="user.enable_attr" :document="document"/></template>
              <el-input v-model="form.user.enable_attr" :disabled="isLocked('user.enable_attr')" placeholder="userAccountControl"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item>
              <template #label><field-label title="启用属性值" field="user.enable_attr_value" :document="document"/></template>
              <el-input v-model="form.user.enable_attr_value" :disabled="isLocked('user.enable_attr_value')" placeholder="AD 使用任意非空值"/>
            </el-form-item>
          </el-col>
        </el-row>
      </el-card>

      <el-card shadow="never">
        <template #header>授权与安全</template>
        <el-row :gutter="16">
          <el-col :xs="24" :md="12">
            <el-form-item>
              <template #label><field-label title="允许登录组" field="user.allow_group" :document="document"/></template>
              <el-input v-model="form.user.allow_group" :disabled="isLocked('user.allow_group')" placeholder="组名或完整 DN"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item>
              <template #label><field-label title="管理员组" field="user.admin_group" :document="document"/></template>
              <el-input v-model="form.user.admin_group" :disabled="isLocked('user.admin_group')" placeholder="组名或完整 DN"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item>
              <template #label><field-label title="支持 AD 嵌套组" field="nested_groups" :document="document"/></template>
              <el-switch v-model="form.nested_groups" :disabled="isLocked('nested_groups')"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item>
              <template #label><field-label title="TLS 证书验证" field="tls_verify" :document="document"/></template>
              <el-switch v-model="form.tls_verify" :disabled="isLocked('tls_verify')"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item>
              <template #label><field-label title="本地管理员应急登录" field="emergency_local_admin" :document="document"/></template>
              <el-switch v-model="form.emergency_local_admin" :disabled="isLocked('emergency_local_admin')"/>
              <div class="field-help">LDAP 失败时只允许本地管理员使用本地密码，普通同名账户不会回退。</div>
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item>
              <template #label><field-label title="TLS CA 文件" field="tls_ca_file" :document="document"/></template>
              <el-input v-model="form.tls_ca_file" :disabled="isLocked('tls_ca_file')" placeholder="容器内 PEM 文件路径；使用系统 CA 时留空"/>
            </el-form-item>
          </el-col>
        </el-row>
      </el-card>

      <el-card shadow="never">
        <template #header>测试用户（可选）</template>
        <el-row :gutter="16">
          <el-col :xs="24" :md="12">
            <el-form-item label="测试用户名">
              <el-input v-model="testUser.username" autocomplete="off" placeholder="jdoe 或 jdoe@example.com"/>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item label="测试用户密码">
              <el-input v-model="testUser.password" type="password" show-password autocomplete="new-password" placeholder="留空时只测试搜索和组权限"/>
            </el-form-item>
          </el-col>
        </el-row>
        <el-timeline v-if="testResult.steps?.length" class="test-result">
          <el-timeline-item
            v-for="step in testResult.steps"
            :key="step.name"
            :type="step.success ? 'success' : 'danger'"
            :timestamp="step.success ? '成功' : '失败'"
          >
            <strong>{{ testStepLabel(step.name) }}</strong>：{{ step.message }}
          </el-timeline-item>
        </el-timeline>
        <el-descriptions v-if="testResult.user" :column="2" border>
          <el-descriptions-item label="用户">{{ testResult.user.username }}</el-descriptions-item>
          <el-descriptions-item label="状态">{{ testResult.user.enabled ? '启用' : '禁用' }}</el-descriptions-item>
          <el-descriptions-item label="允许登录">{{ testResult.is_allowed ? '是' : '否' }}</el-descriptions-item>
          <el-descriptions-item label="管理员">{{ testResult.is_admin ? '是' : '否' }}</el-descriptions-item>
          <el-descriptions-item label="DN" :span="2">{{ testResult.user.dn }}</el-descriptions-item>
        </el-descriptions>
      </el-card>
    </el-form>
  </div>
</template>

<script setup>
import { defineComponent, h, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, ElTag } from 'element-plus'
import {
  getLdapConfig,
  rollbackLdapConfig,
  saveLdapConfig,
  testLdapConfig,
} from '@/api/ldap'

const emptyForm = () => ({
  enable: false,
  url: '',
  tls_ca_file: '',
  tls_verify: false,
  base_dn: '',
  bind_dn: '',
  connect_timeout_seconds: 5,
  operation_timeout_seconds: 10,
  nested_groups: true,
  emergency_local_admin: true,
  user: {
    base_dn: '',
    enable_attr: '',
    enable_attr_value: '',
    filter: '(objectClass=person)',
    username: 'uid',
    email: 'mail',
    first_name: 'givenName',
    last_name: 'sn',
    sync: true,
    admin_group: '',
    allow_group: '',
  },
})

const form = reactive(emptyForm())
const document = reactive({
  locked_fields: [],
  sources: {},
  password_configured: false,
  can_save_password: false,
  can_rollback: false,
})
const bindPassword = ref('')
const clearBindPassword = ref(false)
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const rollingBack = ref(false)
const testUser = reactive({ username: '', password: '' })
const testResult = reactive({ steps: [], user: null })

const attributeFields = [
  { key: 'email', label: '邮箱属性' },
  { key: 'first_name', label: '名字属性' },
  { key: 'last_name', label: '姓氏属性' },
]

const sourceLabels = {
  environment: '环境变量',
  frontend: '页面配置',
  config: 'config.yaml',
}

const FieldLabel = defineComponent({
  props: {
    title: { type: String, required: true },
    field: { type: String, required: true },
    document: { type: Object, required: true },
  },
  setup (props) {
    return () => h('span', { class: 'field-label' }, [
      h('span', props.title),
      h(ElTag, {
        size: 'small',
        type: props.document.sources?.[props.field] === 'environment' ? 'warning' : 'info',
      }, () => sourceLabels[props.document.sources?.[props.field]] || '默认配置'),
    ])
  },
})

const isLocked = field => document.locked_fields?.includes(field)

function applyDocument (payload) {
  Object.assign(document, payload)
  Object.assign(form, emptyForm(), payload.config || {})
  form.user = Object.assign(emptyForm().user, payload.config?.user || {})
  bindPassword.value = ''
  clearBindPassword.value = false
}

async function load () {
  loading.value = true
  try {
    const response = await getLdapConfig()
    applyDocument(response.data)
  } finally {
    loading.value = false
  }
}

function applyActiveDirectoryTemplate () {
  Object.assign(form, {
    enable: true,
    url: 'ldap://dc.example.com:389',
    tls_ca_file: '',
    tls_verify: false,
    base_dn: 'DC=example,DC=com',
    connect_timeout_seconds: 5,
    operation_timeout_seconds: 10,
    nested_groups: true,
    emergency_local_admin: true,
  })
  Object.assign(form.user, {
    base_dn: 'DC=example,DC=com',
    enable_attr: 'userAccountControl',
    enable_attr_value: 'enabled',
    filter: '(&(objectCategory=person)(objectClass=user))',
    username: 'sAMAccountName,userPrincipalName',
    email: 'mail',
    first_name: 'givenName',
    last_name: 'sn',
    sync: true,
    admin_group: 'RustDesk-Admins',
    allow_group: 'RustDesk-Users',
  })
  for (const field of document.locked_fields || []) {
    const path = field.split('.')
    if (path.length === 2 && path[0] === 'user') {
      form.user[path[1]] = document.config?.user?.[path[1]]
    } else if (path.length === 1 && Object.hasOwn(document.config || {}, path[0])) {
      form[path[0]] = document.config?.[path[0]]
    }
  }
  ElMessage.success('已应用通用 AD 模板，请按实际环境修改并补充 Bind DN 和 Bind Password。')
}

async function save () {
  if (form.enable) {
    await ElMessageBox.confirm(
      '保存后新登录会立即使用这套 LDAP 配置。建议先完成“测试连接”。是否继续？',
      '保存 LDAP 配置',
      { confirmButtonText: '保存并应用', cancelButtonText: '取消', type: 'warning' },
    )
  }
  saving.value = true
  try {
    const response = await saveLdapConfig({
      config: form,
      bind_password: bindPassword.value,
      clear_bind_password: clearBindPassword.value,
    })
    applyDocument(response.data)
    ElMessage.success('LDAP 配置已保存并立即应用。')
  } finally {
    saving.value = false
  }
}

async function testConfiguration () {
  testing.value = true
  try {
    const response = await testLdapConfig({
      config: form,
      bind_password: bindPassword.value,
      test_username: testUser.username,
      test_password: testUser.password,
    })
    Object.assign(testResult, { steps: [], user: null, is_admin: false, is_allowed: false }, response.data)
    if (response.data.success) {
      ElMessage.success('LDAP 测试通过。')
    } else {
      ElMessage.error('LDAP 测试未通过，请查看测试步骤。')
    }
  } finally {
    testUser.password = ''
    testing.value = false
  }
}

async function rollback () {
  await ElMessageBox.confirm('将恢复并立即应用上一次有效配置，是否继续？', '恢复 LDAP 配置', {
    confirmButtonText: '恢复',
    cancelButtonText: '取消',
    type: 'warning',
  })
  rollingBack.value = true
  try {
    const response = await rollbackLdapConfig()
    applyDocument(response.data)
    ElMessage.success('已恢复并应用上一次 LDAP 配置。')
  } finally {
    rollingBack.value = false
  }
}

const testStepLabels = {
  validation: '配置校验',
  service_bind: '连接与服务账户 Bind',
  user_search: '用户搜索',
  group_access: '组权限',
  user_bind: '测试用户密码',
}
const testStepLabel = name => testStepLabels[name] || name

onMounted(load)
</script>

<style scoped>
.ldap-page {
  padding: 20px;
}

.notice {
  margin-top: 16px;
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin: 16px 0;
}

.ldap-form {
  display: grid;
  gap: 16px;
}

.card-title,
.field-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.field-label {
  width: 100%;
}

.field-help {
  margin-top: 6px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.5;
}

.clear-password {
  margin-top: 6px;
}

.test-result {
  margin-top: 8px;
}

@media (max-width: 768px) {
  .ldap-page {
    padding: 12px;
  }
}
</style>
