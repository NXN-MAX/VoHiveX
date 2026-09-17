<script setup lang="ts">
import { ref } from 'vue'
import RiIcon from '@/components/RiIcon.vue'
defineProps<{ value?: string; placeholder?: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:value': [value: string] }>()
const visible = ref(false)
</script>
<template><a-input :value="value" :type="visible ? 'text' : 'password'" :placeholder="placeholder" :disabled="disabled" @update:value="emit('update:value', $event)"><template #suffix><button type="button" class="password-eye" :aria-label="visible ? '隐藏密码' : '显示密码'" @click="visible = !visible"><RiIcon :name="visible ? 'eye-off-line' : 'eye-line'" :size="17" /></button></template></a-input></template>
<style scoped>.password-eye{display:grid;width:28px;height:28px;place-items:center;padding:0;border:0;border-radius:50%;background:transparent;color:var(--vx-ink);cursor:pointer}.password-eye:hover{background:var(--vx-neutral)}</style>
