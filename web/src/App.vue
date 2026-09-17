<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ConfigProvider, Modal } from 'antdv-next'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'

const auth = useAuthStore()
const ui = useUiStore()
const agreementText = ref('')
const agreementKey = 'vohive_disclaimer_agreed_at'
const requiredText = '我同意并确认'
const week = 7 * 24 * 60 * 60 * 1000
const showAgreement = ref(false)

watch(
  () => auth.isAuthenticated,
  (signedIn) => {
    if (!signedIn) {
      showAgreement.value = false
      return
    }
    const value = Number(localStorage.getItem(agreementKey) || 0)
    showAgreement.value = !value || Date.now() - value >= week
  },
  { immediate: true },
)

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
    Tabs: { itemActiveColor: '#163300', itemSelectedColor: '#163300', inkBarColor: 'transparent' },
    Modal: { borderRadiusLG: 24 },
    Input: { activeBorderColor: '#163300', hoverBorderColor: '#62645f' },
  },
}))

function acceptAgreement() {
  if (agreementText.value !== requiredText) return
  localStorage.setItem(agreementKey, String(Date.now()))
  showAgreement.value = false
  agreementText.value = ''
}
</script>

<template>
  <ConfigProvider :theme="theme">
    <RouterView />
    <Modal
      v-model:open="showAgreement"
      :closable="false"
      :mask-closable="false"
      :keyboard="false"
      :footer="null"
      width="560px"
      class="agreement-modal"
    >
      <div class="agreement-content">
        <h2>VoHiveX 最终用户许可与免责声明</h2>
        <ol>
          <li>本软件仅供技术研究、学习交流和个人内部测试使用，严禁用于商业用途。</li>
          <li>严禁用于电信诈骗、垃圾短信、非法网络代理及其他违法违规场景。</li>
          <li>Modem 操作可能产生通信资费或设备风险，使用者应自行确认操作并承担相应责任。</li>
          <li>继续使用表示您理解并接受以上条款。</li>
        </ol>
        <label for="agreement-input">请输入“{{ requiredText }}”</label>
        <a-input id="agreement-input" v-model:value="agreementText" size="large" />
        <div class="dialog-actions">
          <a-button type="primary" :disabled="agreementText !== requiredText" @click="acceptAgreement">
            同意并继续
          </a-button>
        </div>
      </div>
    </Modal>
  </ConfigProvider>
</template>
