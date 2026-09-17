import { defineStore } from 'pinia'

export const useUiStore = defineStore('ui', {
  state: () => ({
    dark: localStorage.getItem('theme') === 'dark',
    collapsed: localStorage.getItem('sidebar-collapsed') === '1',
    mobileOpen: false,
  }),
  actions: {
    applyTheme() {
      document.documentElement.classList.toggle('dark', this.dark)
      document.documentElement.style.colorScheme = this.dark ? 'dark' : 'light'
    },
    toggleTheme() {
      this.dark = !this.dark
      localStorage.setItem('theme', this.dark ? 'dark' : 'light')
      this.applyTheme()
    },
    toggleCollapsed() {
      this.collapsed = !this.collapsed
      localStorage.setItem('sidebar-collapsed', this.collapsed ? '1' : '0')
    },
  },
})
