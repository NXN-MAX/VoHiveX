<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ConfigProvider, Modal } from 'antdv-next'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { apiError, request } from '@/api/http'

const auth = useAuthStore()
const ui = useUiStore()
const agreementText = ref('')
const legacyAgreementKey = 'vohive_disclaimer_agreed_at'
const requiredText = '我同意并确认'
const showAgreement = ref(false)
const agreementBusy = ref(false)
const agreementError = ref('')
const agreementUserID = ref('')
let agreementRequest = 0
interface AgreementState { user_id: string; accepted: boolean; accepted_at: number }

async function loadAgreement() {
  const current = ++agreementRequest
  const token = auth.token
  showAgreement.value = false
  agreementError.value = ''
  agreementText.value = ''
  agreementUserID.value = ''
  if (!token) { agreementBusy.value = false; return }
  agreementBusy.value = true
  try {
    let state = await request<AgreementState>({ url: '/settings/agreement' })
    if (current !== agreementRequest || token !== auth.token) return
    if (!state.accepted) {
      let previous = 0
      try { previous = Number(localStorage.getItem(legacyAgreementKey) || 0) } catch { /* Storage may be disabled. */ }
      if (Number.isSafeInteger(previous) && previous >= 946684800000 && previous <= Date.now()) {
        state = await request<AgreementState>({ method: 'POST', url: '/settings/agreement', data: { user_id: state.user_id, previous_accepted_at: previous } })
        if (current !== agreementRequest || token !== auth.token) return
      }
    }
    agreementUserID.value = state.user_id
    showAgreement.value = !state.accepted
    if (state.accepted) {
      try { localStorage.removeItem(legacyAgreementKey) } catch { /* Server state is authoritative. */ }
    }
  } catch (reason) {
    if (current === agreementRequest && token === auth.token) {
      const error = apiError(reason)
      if (error.status === 401) auth.logout()
      else agreementError.value = error.message
    }
  } finally {
    if (current === agreementRequest) agreementBusy.value = false
  }
}

watch(() => auth.token, () => { void loadAgreement() }, { immediate: true })

const theme = computed(() => ({
  token: {
    colorPrimary: '#3b651c',
    colorInfo: '#3b651c',
    colorSuccess: '#50a41d',
    colorText: ui.dark ? '#dfdfd6' : '#163300',
    colorTextSecondary: ui.dark ? '#98989f' : '#62645f',
    colorBgBase: ui.dark ? '#1b1b1f' : '#ffffff',
    colorBgContainer: ui.dark ? '#202127' : '#ffffff',
    colorBorder: ui.dark ? '#3c3f44' : '#deded9',
    borderRadius: 12,
    borderRadiusLG: 20,
    controlHeight: 40,
    fontFamily: 'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  },
  components: {
    Button: { borderRadius: 999, primaryShadow: 'none' },
    Tooltip: { borderRadius: 6, controlHeight: 32 },
    Tabs: { itemActiveColor: '#163300', itemSelectedColor: '#163300', inkBarColor: 'transparent' },
    Modal: { borderRadiusLG: 24 },
    Input: { activeBorderColor: '#163300', hoverBorderColor: '#62645f' },
  },
}))

async function acceptAgreement() {
  if (agreementText.value !== requiredText || agreementBusy.value || !agreementUserID.value) return
  const current = agreementRequest
  const token = auth.token
  agreementBusy.value = true
  agreementError.value = ''
  try {
    const state = await request<AgreementState>({ method: 'POST', url: '/settings/agreement', data: { user_id: agreementUserID.value, confirmation: agreementText.value } })
    if (current !== agreementRequest || token !== auth.token) return
    showAgreement.value = !state.accepted
    if (state.accepted) {
      agreementText.value = ''
      try { localStorage.removeItem(legacyAgreementKey) } catch { /* Server state is authoritative. */ }
    }
  } catch (reason) {
    if (current === agreementRequest && token === auth.token) {
      const error = apiError(reason)
      if (error.status === 401) auth.logout()
      else agreementError.value = error.message
    }
  } finally {
    if (current === agreementRequest) agreementBusy.value = false
  }
}

</script>

<template>
  <ConfigProvider :theme="theme">
    <RouterView />
    <Modal
      :open="auth.isAuthenticated && (showAgreement || Boolean(agreementError))"
      :closable="false"
      :mask-closable="false"
      :keyboard="false"
      :footer="null"
      width="560px"
      class="agreement-modal"
    >
      <div v-if="!showAgreement && agreementError" class="agreement-content">
        <h2>暂时无法读取确认状态</h2>
        <p role="alert">{{ agreementError }}</p>
        <div class="dialog-actions"><a-button :loading="agreementBusy" @click="loadAgreement">重试</a-button></div>
      </div>
      <div v-else class="agreement-content">
        <h2>VoHiveX 最终用户许可与免责声明</h2>
        <ol>
          <li>本软件仅供技术研究、学习交流和个人内部测试使用，严禁用于商业用途。</li>
          <li>严禁用于电信诈骗、垃圾短信、非法网络代理及其他违法违规场景。</li>
          <li>Modem 操作可能产生通信资费或设备风险，使用者应自行确认操作并承担相应责任。</li>
          <li>继续使用表示您理解并接受以上条款。</li>
        </ol>
        <label for="agreement-input">请输入“{{ requiredText }}”</label>
        <a-input id="agreement-input" v-model:value="agreementText" size="large" :disabled="agreementBusy" />
        <p v-if="agreementError" role="alert">{{ agreementError }}</p>
        <div class="dialog-actions">
          <a-button type="primary" :disabled="agreementText !== requiredText" :loading="agreementBusy" @click="acceptAgreement">
            同意并继续
          </a-button>
        </div>
      </div>
    </Modal>
  </ConfigProvider>
</template>
