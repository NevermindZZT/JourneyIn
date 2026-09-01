/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_JOURNEYIN_VERSION: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
