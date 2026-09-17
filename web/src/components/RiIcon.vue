<script setup lang="ts">
withDefaults(defineProps<{ name: string; size?: number | string; label?: string }>(), {
  size: 20,
  label: '',
})
</script>

<template>
  <span
    class="ri-icon"
    :style="{
      width: typeof size === 'number' ? `${size}px` : size,
      height: typeof size === 'number' ? `${size}px` : size,
      maskImage: `url(/icons/${name}.svg)`,
      WebkitMaskImage: `url(/icons/${name}.svg)`,
    }"
    :aria-label="label || undefined"
    :aria-hidden="label ? undefined : 'true'"
  />
</template>
