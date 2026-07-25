import type { RelayApi } from './index'

declare global {
  interface Window {
    relay: RelayApi
  }
}

export {}
