<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { message } from 'antdv-next'
import PageHeader from '@/components/PageHeader.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import { apiError, request } from '@/api/http'

const active = ref('telegram')
const loading = ref(false)
const saving = ref(false)
const config = reactive<any>({
  telegram: { enabled: false, bot_token: '', chat_id: '', admin_id: '', base_url: '', proxy: '' },
  feishu: { enabled: false, app_id: '', app_secret: '', chat_ids: '' },
  qq: { enabled: false, app_id: '', app_secret: '', group_ids: '', direct_ids: '' },
  bark: { enabled: false, urls: '', group: 'vohive', icon: '', level: 'active' },
  email: { enabled: false, use_ssl: false, smtp_host: '', smtp_port: 465, username: '', password: '', from_address: '', to_addresses: '' },
  pushplus: { enabled: false, token: '', topic: '', channel: 'wechat' },
  webhook: { enabled: false, urls: '', secret: '', headers_text: '', timeout_ms: 5000, retry_max: 3, text_template: '{{device_label}} {{text}}' },
})
const tabs = [{ key: 'telegram', label: 'Telegram Bot' }, { key: 'feishu', label: '飞书 Bot' }, { key: 'qq', label: 'QQ Bot' }, { key: 'bark', label: 'Bark' }, { key: 'email', label: 'Email' }, { key: 'pushplus', label: 'Pushplus' }, { key: 'webhook', label: 'Webhook' }]
const barkLevels = [{ label: '时效性 (timeSensitive)', value: 'timeSensitive' }, { label: '积极 (active)', value: 'active' }, { label: '被动 (passive)', value: 'passive' }]
const pushplusChannels = [{ label: '微信 (wechat)', value: 'wechat' }, { label: 'Webhook (webhook)', value: 'webhook' }, { label: '企业微信 (cp)', value: 'cp' }, { label: '邮件 (mail)', value: 'mail' }]
const webhookTemplatePlaceholder = '{{device_label}} {{text}}'
const webhookTemplateHelp = '支持 {{text}}、{{event}}、{{timestamp}}、{{device_id}}、{{device_name}}、{{device_label}}；留空则发送原始文本。'

function join(value: unknown) { return Array.isArray(value) ? value.join(',') : String(value || '') }
function lines(value: unknown) { return Array.isArray(value) ? value.join('\n') : String(value || '') }
function split(value: unknown) { return String(value || '').split(/[\n,]/).map((row) => row.trim()).filter(Boolean) }
function headersToText(value: unknown) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return ''
  return Object.entries(value as Record<string, unknown>).map(([key, item]) => `${key}: ${String(item)}`).join('\n')
}
function parseHeaders(value: unknown) {
  const headers: Record<string, string> = {}
  for (const row of String(value || '').split('\n')) {
    const index = row.indexOf(':')
    if (index <= 0) continue
    const key = row.slice(0, index).trim()
    const item = row.slice(index + 1).trim()
    if (key && item) headers[key] = item
  }
  return headers
}

async function load() {
  loading.value = true
  try {
    const data = await request<any>({ url: '/settings/notifications' })
    for (const key of Object.keys(config)) if (data[key]) Object.assign(config[key], data[key])
    config.feishu.chat_ids = join(data.feishu?.chat_ids)
    config.bark.urls = lines(data.bark?.urls)
    config.email.to_addresses = join(data.email?.to_addresses)
    config.webhook.urls = lines(data.webhook?.urls)
    config.webhook.headers_text = headersToText(data.webhook?.headers)
  } catch (reason) { message.error(apiError(reason).message) } finally { loading.value = false }
}
function payload() {
  const { headers_text: _headersText, ...webhook } = config.webhook
  return {
    ...config,
    telegram: { ...config.telegram, chat_id: Number(config.telegram.chat_id) || 0, admin_id: Number(config.telegram.admin_id) || 0 },
    feishu: { ...config.feishu, chat_ids: split(config.feishu.chat_ids) },
    bark: { ...config.bark, urls: split(config.bark.urls) },
    email: { ...config.email, smtp_port: Number(config.email.smtp_port) || 0, to_addresses: split(config.email.to_addresses) },
    webhook: { ...webhook, urls: split(config.webhook.urls), timeout_ms: Number(config.webhook.timeout_ms), retry_max: Number(config.webhook.retry_max), headers: parseHeaders(config.webhook.headers_text) },
  }
}
async function save() {
  saving.value = true
  try { await request({ method: 'PUT', url: '/settings/notifications', data: payload() }); message.success('消息推送配置已保存') }
  catch (reason) { message.error(apiError(reason).message) }
  finally { saving.value = false }
}
async function test(kind: 'webhook' | 'bark' | 'email') {
  try { await request({ method: 'POST', url: `/settings/notifications/${kind}/test`, data: payload()[kind] }); message.success('测试消息已发送') }
  catch (reason) { message.error(apiError(reason).message) }
}
onMounted(load)
</script>

<template>
  <div class="push-page">
    <PageHeader title="消息推送" subtitle="管理消息推送渠道与通知内容">
      <template #actions><a-button type="primary" :loading="saving" @click="save">保存配置</a-button></template>
    </PageHeader>
    <section class="push-panel">
      <a-spin :spinning="loading">
        <a-tabs v-model:active-key="active" class="push-tabs">
          <a-tab-pane v-for="tab in tabs" :key="tab.key" :tab="tab.label">
            <div class="channel-heading"><div><h2>{{ tab.label }}</h2><p>相关凭证仅保存在本机配置文件中</p></div><a-switch v-model:checked="config[tab.key].enabled" /></div>

            <a-form v-if="tab.key === 'telegram'" layout="vertical" class="channel-form">
              <a-form-item label="Bot Token"><PasswordInput v-model:value="config.telegram.bot_token" :disabled="!config.telegram.enabled" placeholder="xxxx:yyyy" /></a-form-item>
              <div class="two-cols"><a-form-item label="Chat ID"><a-input v-model:value="config.telegram.chat_id" :disabled="!config.telegram.enabled" placeholder="例如 123456" /></a-form-item><a-form-item label="Admin ID"><a-input v-model:value="config.telegram.admin_id" :disabled="!config.telegram.enabled" placeholder="例如 123456" /></a-form-item></div>
              <a-form-item label="TG API 反代（可选）"><a-input v-model:value="config.telegram.base_url" :disabled="!config.telegram.enabled" placeholder="留空直连 api.telegram.org；需要反代时填写" /><p class="field-help">反向代理地址（例如 https://api.telegram.org/bot%s/%s）</p></a-form-item>
              <a-form-item label="HTTP 代理（可选）"><a-input v-model:value="config.telegram.proxy" :disabled="!config.telegram.enabled" placeholder="例如 http://127.0.0.1:7890" /><p class="field-help">用于连接 API 服务器的 HTTP 代理</p></a-form-item>
            </a-form>

            <a-form v-else-if="tab.key === 'feishu'" layout="vertical" class="channel-form">
              <div class="two-cols"><a-form-item label="App ID"><a-input v-model:value="config.feishu.app_id" :disabled="!config.feishu.enabled" placeholder="cli_xxxx" /></a-form-item><a-form-item label="App Secret"><PasswordInput v-model:value="config.feishu.app_secret" :disabled="!config.feishu.enabled" placeholder="••••••••" /></a-form-item></div>
              <a-form-item label="Chat IDs"><a-input v-model:value="config.feishu.chat_ids" :disabled="!config.feishu.enabled" placeholder="多个群组用英文逗号分隔" /><p class="field-help">飞书群聊的 Chat ID（oc_xxxx），可通过飞书开放平台 API 获取，支持逗号分隔多个群组。</p></a-form-item>
              <ol class="config-notes"><li>在飞书开放平台创建自建应用并启用“机器人”能力。</li><li>在“事件与回调 → 事件配置”中选择“使用长连接接收事件”。</li><li>添加 im:message 和 im:message:send_as_bot 权限。</li></ol>
            </a-form>

            <a-form v-else-if="tab.key === 'qq'" layout="vertical" class="channel-form">
              <div class="two-cols"><a-form-item label="App ID"><a-input v-model:value="config.qq.app_id" :disabled="!config.qq.enabled" placeholder="QQ Bot App ID" /></a-form-item><a-form-item label="App Secret"><PasswordInput v-model:value="config.qq.app_secret" :disabled="!config.qq.enabled" placeholder="••••••••" /></a-form-item></div>
              <a-form-item label="Group IDs（群聊）"><a-input v-model:value="config.qq.group_ids" :disabled="!config.qq.enabled" placeholder="群聊 OpenID，多个使用逗号分隔" /></a-form-item>
              <a-form-item label="User IDs（私聊）"><a-input v-model:value="config.qq.direct_ids" :disabled="!config.qq.enabled" placeholder="用户 OpenID，多个使用逗号分隔" /></a-form-item>
              <ol class="config-notes"><li>在 QQ Bot 官方控制台申请并配置机器人。</li><li>向机器人发送消息后，可从系统日志查看 OpenID；填写后 Bot 只对匹配会话回复和推送。</li></ol>
            </a-form>

            <a-form v-else-if="tab.key === 'bark'" layout="vertical" class="channel-form">
              <a-form-item label="目标 URLs"><a-textarea v-model:value="config.bark.urls" :disabled="!config.bark.enabled" :rows="3" placeholder="https://api.day.app/YOUR_KEY/&#10;每行一个地址" /></a-form-item>
              <div class="two-cols"><a-form-item label="分组（Group）"><a-input v-model:value="config.bark.group" :disabled="!config.bark.enabled" placeholder="例如 vohive" /><p class="field-help">iOS 设备上的通知分组。</p></a-form-item><a-form-item label="通知级别（Level）"><a-select v-model:value="config.bark.level" :disabled="!config.bark.enabled" :options="barkLevels" placeholder="选择通知级别" /><p class="field-help">iOS 的专注模式和打扰规则会根据此级别决定是否亮屏。</p></a-form-item></div>
              <a-form-item label="图标（Icon）"><a-input v-model:value="config.bark.icon" :disabled="!config.bark.enabled" placeholder="图标 URL，可选" /></a-form-item>
              <a-button class="ghost-button" :disabled="!config.bark.enabled || !config.bark.urls.trim()" @click="test('bark')">发送测试</a-button>
            </a-form>

            <a-form v-else-if="tab.key === 'email'" layout="vertical" class="channel-form">
              <div class="three-cols"><a-form-item label="SMTP 主机"><a-input v-model:value="config.email.smtp_host" :disabled="!config.email.enabled" placeholder="smtp.example.com" /></a-form-item><a-form-item label="SMTP 端口"><a-input-number v-model:value="config.email.smtp_port" :disabled="!config.email.enabled" placeholder="465 / 587" /></a-form-item><a-form-item label="使用 SSL/TLS"><div class="switch-field"><a-switch v-model:checked="config.email.use_ssl" :disabled="!config.email.enabled" /></div></a-form-item></div>
              <div class="two-cols"><a-form-item label="用户名（Username）"><a-input v-model:value="config.email.username" :disabled="!config.email.enabled" placeholder="邮箱账号" /></a-form-item><a-form-item label="密码（Password）"><PasswordInput v-model:value="config.email.password" :disabled="!config.email.enabled" placeholder="邮箱密码或授权码" /></a-form-item></div>
              <div class="two-cols"><a-form-item label="发件人地址（From）"><a-input v-model:value="config.email.from_address" :disabled="!config.email.enabled" placeholder="例如 noreply@example.com" /></a-form-item><a-form-item label="收件人地址（To）"><a-input v-model:value="config.email.to_addresses" :disabled="!config.email.enabled" placeholder="多个收件人请用英文逗号分隔" /></a-form-item></div>
              <a-button class="ghost-button" :disabled="!config.email.enabled" @click="test('email')">发送测试</a-button>
            </a-form>

            <a-form v-else-if="tab.key === 'pushplus'" layout="vertical" class="channel-form">
              <a-form-item label="Token"><PasswordInput v-model:value="config.pushplus.token" :disabled="!config.pushplus.enabled" placeholder="Pushplus 用户的 Token" /></a-form-item>
              <div class="two-cols"><a-form-item label="群组编码（Topic）"><a-input v-model:value="config.pushplus.topic" :disabled="!config.pushplus.enabled" placeholder="群组编码，不填则发给个人" /></a-form-item><a-form-item label="渠道（Channel）"><a-select v-model:value="config.pushplus.channel" :disabled="!config.pushplus.enabled" :options="pushplusChannels" placeholder="选择渠道" /></a-form-item></div>
            </a-form>

            <a-form v-else layout="vertical" class="channel-form">
              <a-form-item label="目标 URLs"><a-textarea v-model:value="config.webhook.urls" :disabled="!config.webhook.enabled" :rows="3" placeholder="https://...&#10;每行一个地址" /></a-form-item>
              <a-form-item label="数字签名密钥（Secret）"><PasswordInput v-model:value="config.webhook.secret" :disabled="!config.webhook.enabled" placeholder="用于 HMAC-SHA256 签名，选填" /><p class="field-help">配置后通过请求头 X-Vohive-Signature 提供 payload 验证。</p></a-form-item>
              <a-form-item label="自定义请求头（Headers）"><a-textarea v-model:value="config.webhook.headers_text" :disabled="!config.webhook.enabled" :rows="3" placeholder="Authorization: Bearer xxx&#10;X-Api-Key: your-key" /><p class="field-help">每行填写一个 Header。Content-Type 与 X-Vohive-Signature 为系统保留头。</p></a-form-item>
              <a-form-item label="文本模板（Text Template）"><a-textarea v-model:value="config.webhook.text_template" :disabled="!config.webhook.enabled" :rows="2" :placeholder="webhookTemplatePlaceholder" /><p class="field-help">{{ webhookTemplateHelp }}</p></a-form-item>
              <div class="two-cols"><a-form-item label="请求超时（ms）"><a-input-number v-model:value="config.webhook.timeout_ms" :disabled="!config.webhook.enabled" :min="1000" :max="60000" /></a-form-item><a-form-item label="最大重试次数"><a-input-number v-model:value="config.webhook.retry_max" :disabled="!config.webhook.enabled" :min="0" :max="10" /></a-form-item></div>
              <a-button class="ghost-button" :disabled="!config.webhook.enabled || !config.webhook.urls.trim()" @click="test('webhook')">发送测试</a-button>
            </a-form>
          </a-tab-pane>
        </a-tabs>
      </a-spin>
    </section>
  </div>
</template>

<style scoped>
.push-page,.push-panel,.channel-form{width:100%;min-width:0}.push-panel{padding:16px 24px 28px;border-radius:24px;background:var(--vx-canvas)}.push-tabs :deep(.ant-tabs-tab){margin-right:4px!important;padding:9px 12px!important;font-size:13px}.channel-heading{display:flex;align-items:center;justify-content:space-between;margin:0 0 20px}.channel-heading h2{margin:0;color:var(--vx-ink);font-size:20px}.channel-heading p{margin:4px 0 0;color:var(--vx-muted);font-size:12px}.two-cols,.three-cols{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.three-cols{grid-template-columns:minmax(0,1.3fr) minmax(150px,.55fr) minmax(150px,.55fr)}.channel-form :deep(.ant-input-number){width:100%}.field-help{margin:6px 2px 0;color:var(--vx-muted);font-size:11px;line-height:1.55}.config-notes{display:grid;gap:5px;margin:0 0 20px;padding-left:22px;color:var(--vx-muted);font-size:12px;line-height:1.55}.switch-field{display:flex;height:42px;align-items:center;padding:0 3px}@media(max-width:760px){.two-cols,.three-cols{grid-template-columns:1fr}}
</style>
