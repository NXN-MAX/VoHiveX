<script setup lang="ts">
import RiIcon from './RiIcon.vue'
withDefaults(defineProps<{ title: string; description?: string; icon?: string }>(), { icon: 'inbox-line' })
</script>

<template>
  <div class="empty-state">
    <RiIcon :name="icon" :size="28" />
    <strong>{{ title }}</strong>
    <p v-if="description">{{ description }}</p>
  </div>
</template>
