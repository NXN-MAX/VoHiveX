/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}

declare module 'jsqr' {
  export interface QRCode {
    data: string
  }
  export default function jsQR(
    data: Uint8ClampedArray,
    width: number,
    height: number,
  ): QRCode | null
}
