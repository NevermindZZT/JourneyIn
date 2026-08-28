import type { CapacitorConfig } from '@capacitor/cli'

const config: CapacitorConfig = {
  appId: 'app.journeyin.client',
  appName: 'JourneyIn',
  webDir: '../web/dist',
  bundledWebRuntime: false,
  server: {
    cleartext: false,
  },
}

export default config
