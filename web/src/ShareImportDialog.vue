<script setup lang="ts">
import { computed, ref, watch } from 'vue'

type ShareTripDocument = { id?: string; title?: string; status?: string; date_range?: { start?: string; end?: string }; days?: Array<{ stops?: unknown[] }> }
type ShareItem = { trip_id: string; content_hash: string; trip: ShareTripDocument }
type SharePackage = { format?: string; bundle_schema_version?: number; source_origin?: string; items: ShareItem[] }
type PreviewItem = { source_trip_id: string; content_hash: string; title: string; status: string; start_date: string; end_date: string; days: number; stops: number; already_imported: boolean; warnings?: Array<{ path: string; message: string }> }
type PreviewResponse = { preview_id?: string; confirmation_token?: string; requires_confirmation: boolean; expires_at?: string; items: PreviewItem[]; error?: { message?: string } }
type CommitResponse = { status: string; items: Array<{ id: string; title: string }>; error?: { message?: string } }

const props = defineProps<{
  open: boolean
  apiFetch: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
}>()
const emit = defineEmits<{ close: []; imported: [] }>()
const fileInput = ref<HTMLInputElement | null>(null)
const link = ref('')
const sourceOrigin = ref('')
const items = ref<ShareItem[]>([])
const selectedIDs = ref<string[]>([])
const preview = ref<PreviewResponse | null>(null)
const error = ref('')
const busy = ref(false)
const loading = ref(false)
const done = ref<CommitResponse | null>(null)
const commitKey = ref('')
const selectedItems = computed(() => items.value.filter(item => selectedIDs.value.includes(item.trip_id)))
const previewByID = computed(() => new Map((preview.value?.items || []).map(item => [item.source_trip_id, item])))

watch(() => props.open, opened => {
  if (opened) reset()
})
function reset() {
  link.value = ''; sourceOrigin.value = ''; items.value = []; selectedIDs.value = []; preview.value = null; error.value = ''; busy.value = false; loading.value = false; done.value = null; commitKey.value = ''
}
function close() { if (!busy.value && !loading.value) emit('close') }
function pickFile() { fileInput.value?.click() }

function packageFromValue(value: unknown, origin: string): SharePackage {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('分享文件格式不正确')
  const object = value as Record<string, unknown>
  let shareItems: ShareItem[]
  if (object.format === 'journeyin.share-bundle' && object.bundle_schema_version === 1 && Array.isArray(object.items)) {
    if (object.items.length < 1 || object.items.length > 50) throw new Error('分享包行程数量不符合限制（1–50）')
    shareItems = object.items.map((raw, index) => {
      if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('第 ' + (index + 1) + ' 条行程格式不正确')
      const item = raw as Record<string, unknown>
      const trip = item.trip as ShareTripDocument
      const tripID = typeof item.trip_id === 'string' ? item.trip_id : trip?.id
      if (!trip || typeof trip !== 'object' || !tripID || trip.id !== tripID) throw new Error('第 ' + (index + 1) + ' 条行程缺少有效 ID')
      return { trip_id: tripID, content_hash: typeof item.content_hash === 'string' ? item.content_hash : '', trip }
    })
  } else if (typeof object.id === 'string' && object.id && object.schema_version === 1) {
    shareItems = [{ trip_id: object.id, content_hash: '', trip: object as ShareTripDocument }]
  } else {
    throw new Error('不是受支持的 JourneyIn 分享包或行程 JSON')
  }
  const ids = new Set<string>()
  for (const item of shareItems) {
    if (ids.has(item.trip_id)) throw new Error('分享包包含重复行程 ID')
    ids.add(item.trip_id)
  }
  return { format: 'journeyin.share-bundle', bundle_schema_version: 1, source_origin: origin, items: shareItems }
}
function applyPackage(value: unknown, origin: string) {
  const parsed = packageFromValue(value, origin)
  sourceOrigin.value = parsed.source_origin || 'local-file://import'
  items.value = parsed.items
  selectedIDs.value = parsed.items.map(item => item.trip_id)
  preview.value = null
  done.value = null
  error.value = ''
}

async function readBoundedResponse(response: Response) {
  const limit = 17 * 1024 * 1024
  const reader = response.body?.getReader()
  if (!reader) { const text = await response.text(); if (text.length > limit) throw new Error('分享包超过 16 MiB 限制'); return text }
  const decoder = new TextDecoder()
  let text = ''
  let total = 0
  while (true) {
    const chunk = await reader.read()
    if (chunk.done) break
    total += chunk.value.byteLength
    if (total > limit) { void reader.cancel(); throw new Error('分享包超过 16 MiB 限制') }
    text += decoder.decode(chunk.value, { stream: true })
  }
  return text + decoder.decode()
}

async function loadFromLink() {
  if (loading.value || busy.value) return
  error.value = ''; preview.value = null; done.value = null
  let source: URL
  try {
    source = new URL(link.value.trim())
    if (source.username || source.password || source.search || source.hash) throw new Error('链接不能包含账号、密码、查询参数或片段')
    if (!['http:', 'https:'].includes(source.protocol)) throw new Error('只支持 HTTP(S) 分享链接')
    const localHost = source.hostname === 'localhost' || source.hostname === '127.0.0.1' || source.hostname === '[::1]'
    if (source.protocol === 'http:' && !localHost) throw new Error('外部分享源必须使用 HTTPS；本机 HTTP 链接可在本机调试时使用')
    if (source.protocol === 'http:' && window.location.protocol === 'https:') throw new Error('当前页面使用 HTTPS，不能读取不安全的 HTTP 分享源')
    const sharePath = source.pathname.replace(/\.json$/, '')
    if (!/^\/s\/[^/]+$/.test(sharePath)) throw new Error('请输入 JourneyIn 分享链接（/s/…）')
    source.pathname = sharePath.endsWith('.json') ? sharePath : sharePath + '.json'
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '分享链接无效'; return }
  loading.value = true
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), 20000)
  try {
    const response = await fetch(source.toString(), { method: 'GET', credentials: 'omit', redirect: 'error', cache: 'no-store', signal: controller.signal })
    if (!response.ok) throw new Error(response.status === 404 ? '分享链接无效、已过期或已撤销' : '读取分享内容失败（HTTP ' + response.status + '）')
    const declared = Number(response.headers.get('Content-Length') || 0)
    if (declared > 17 * 1024 * 1024) throw new Error('分享包超过 16 MiB 限制')
    const text = await readBoundedResponse(response)
    applyPackage(JSON.parse(text), source.origin)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '读取分享内容失败；请确认来源实例可通过 HTTPS 访问' }
  finally { window.clearTimeout(timeout); loading.value = false }
}
async function readFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  error.value = ''; preview.value = null; done.value = null
  try {
    if (file.size > 17 * 1024 * 1024) throw new Error('分享文件超过 16 MiB 限制')
    const value = JSON.parse(await file.text()) as { source_origin?: string }
    applyPackage(value, typeof value?.source_origin === 'string' && value.source_origin ? value.source_origin : 'local-file://import')
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '无法读取分享文件' }
  finally { input.value = '' }
}
function toggleItem(id: string) {
  if (preview.value) preview.value = null
  selectedIDs.value = selectedIDs.value.includes(id) ? selectedIDs.value.filter(value => value !== id) : [...selectedIDs.value, id]
}
function itemTitle(item: ShareItem) { return item.trip.title || '未命名行程' }
function itemDates(item: ShareItem) { return (item.trip.date_range?.start || '日期未设置') + ' — ' + (item.trip.date_range?.end || '日期未设置') }

async function previewImport() {
  if (busy.value || selectedItems.value.length === 0) return
  busy.value = true; error.value = ''; preview.value = null
  try {
    const response = await props.apiFetch('/api/v1/share-import/previews', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ source_origin: sourceOrigin.value, items: selectedItems.value }) })
    const payload = await response.json() as PreviewResponse
    if (!response.ok) throw new Error(payload.error?.message || '校验分享包失败')
    preview.value = payload
    commitKey.value = typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : 'share-import-' + Date.now() + '-' + Math.random().toString(36).slice(2)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '校验分享包失败' }
  finally { busy.value = false }
}
async function commitImport() {
  if (!preview.value?.preview_id || !preview.value.confirmation_token || busy.value) return
  busy.value = true; error.value = ''
  try {
    const response = await props.apiFetch('/api/v1/share-import/commits', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ preview_id: preview.value.preview_id, confirmation_token: preview.value.confirmation_token, idempotency_key: commitKey.value }) })
    const payload = await response.json() as CommitResponse
    if (!response.ok) throw new Error(payload.error?.message || '导入失败')
    done.value = payload
    emit('imported')
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '导入失败' }
  finally { busy.value = false }
}
</script>

<template>
  <div v-if="open" class="share-import-backdrop" @click.self="close">
    <section class="share-import-dialog" role="dialog" aria-modal="true" aria-labelledby="share-import-title">
      <header><div><p class="import-eyebrow">IMPORT FROM ANOTHER INSTANCE</p><h2 id="share-import-title">从分享导入行程</h2><p>导入为独立副本，不会覆盖现有行程或与来源持续同步。</p></div><button type="button" class="modal-close" aria-label="关闭" @click="close">×</button></header>
      <input ref="fileInput" class="import-file-input" type="file" accept="application/json,.json" @change="readFile" />
      <div v-if="!items.length" class="import-source-form">
        <label>JourneyIn 分享链接<input v-model="link" type="url" autocomplete="url" placeholder="https://journeyin.example/s/…" @keydown.enter.prevent="loadFromLink" /></label>
        <div class="import-source-actions"><button class="import-secondary" type="button" :disabled="loading || !link.trim()" @click="loadFromLink">{{ loading ? '正在读取…' : '读取分享链接' }}</button><button class="import-secondary" type="button" @click="pickFile">选择分享 JSON 文件</button></div>
        <p class="import-privacy">分享链接由 A 实例托管；导入时只读取该只读快照，不会把 B 的登录凭据发送给 A。两实例需网络可达；本机或内网 HTTP 在 HTTPS 页面中可能无法读取。</p>
      </div>
      <template v-else>
        <div class="import-source-summary"><strong>来源：{{ sourceOrigin === 'local-file://import' ? '本地分享文件' : sourceOrigin }}</strong><small>共 {{ items.length }} 条行程 · 选择需要导入的项目</small></div>
        <div class="import-item-list" role="group" aria-label="选择导入的行程">
          <label v-for="item in items" :key="item.trip_id" class="import-item-row">
            <input type="checkbox" :checked="selectedIDs.includes(item.trip_id)" :disabled="!!preview" @change="toggleItem(item.trip_id)" />
            <span class="import-item-copy"><strong>{{ itemTitle(item) }}</strong><small>{{ itemDates(item) }} · {{ item.trip.days?.length || 0 }} 天</small><small v-if="previewByID.get(item.trip_id)?.already_imported" class="import-duplicate">已导入过此快照，将跳过</small></span>
          </label>
        </div>
        <div v-if="preview" class="import-preview-box" role="status">
          <strong>{{ preview.requires_confirmation ? '校验完成，确认后导入新副本' : '所选行程均已导入过' }}</strong>
          <div v-for="item in preview.items" :key="item.source_trip_id"><p>{{ item.title }} · {{ item.already_imported ? '已存在，跳过' : '将作为新副本导入' }}</p><small v-for="warning in item.warnings || []" :key="warning.path + warning.message">{{ warning.path }}：{{ warning.message }}</small></div>
          <small v-if="preview.expires_at">预览有效至 {{ new Date(preview.expires_at).toLocaleTimeString() }}</small>
        </div>
      </template>
      <p v-if="error" class="import-error" role="alert">{{ error }}</p>
      <div v-if="done" class="import-success" role="status">已导入 {{ done.items.length }} 条行程副本。</div>
      <footer><button type="button" class="import-secondary" :disabled="busy || loading" @click="close">{{ done ? '完成' : '取消' }}</button><button v-if="items.length && !preview && !done" type="button" class="import-primary" :disabled="busy || !selectedIDs.length" @click="previewImport">{{ busy ? '正在校验…' : '校验并预览' }}</button><button v-if="preview?.requires_confirmation && !done" type="button" class="import-primary" :disabled="busy" @click="commitImport">{{ busy ? '正在导入…' : '确认导入（' + preview.items.filter(item => !item.already_imported).length + '）' }}</button></footer>
    </section>
  </div>
</template>

<style scoped>
.share-import-backdrop{position:fixed;inset:0;z-index:1210;display:grid;place-items:center;padding:20px;background:#101916a6}.share-import-dialog{position:relative;width:min(660px,100%);max-height:min(88vh,850px);overflow:auto;padding:26px;border-radius:24px;background:var(--surface,#fff);color:var(--on-surface,#19231f);box-shadow:0 24px 70px #0003}.share-import-dialog header{display:flex;justify-content:space-between;gap:16px}.share-import-dialog h2{margin:2px 0 6px;font-size:24px}.share-import-dialog header p{margin:0;color:#66736d}.import-eyebrow{font-size:11px;letter-spacing:.12em;font-weight:700;color:#357567!important}.import-close{border:0;background:transparent;color:inherit;font-size:26px;cursor:pointer;align-self:flex-start}.import-source-form{display:grid;gap:12px;margin-top:22px}.import-source-form label{display:grid;gap:7px;font-size:13px;font-weight:650}.import-source-form input{min-height:46px;padding:0 12px;border:1px solid #c8d2cd;border-radius:12px;font:inherit}.import-source-actions{display:flex;gap:8px;flex-wrap:wrap}.import-privacy{margin:0;padding:12px 14px;border-radius:12px;background:#f3f7f5;color:#596760;font-size:13px;line-height:1.5}.import-file-input{display:none}.import-source-summary{display:grid;gap:4px;margin:20px 0 12px}.import-source-summary small{color:#67746e}.import-item-list{max-height:300px;overflow:auto;border:1px solid #dce4e0;border-radius:14px}.import-item-row{display:flex;align-items:flex-start;gap:12px;padding:13px;border-bottom:1px solid #edf1ef;cursor:pointer}.import-item-row:last-child{border-bottom:0}.import-item-row input{width:18px;height:18px;margin-top:2px;accent-color:#176b59}.import-item-copy{display:grid;gap:4px}.import-item-copy small{font-size:12px;color:#63716b}.import-item-copy .import-duplicate{color:#8a6500}.import-preview-box{margin-top:14px;padding:14px;border-radius:14px;background:#eef6f2}.import-preview-box p{margin:6px 0;font-size:13px}.import-preview-box small{color:#63716b}.import-error{color:#a22b2b;font-size:13px}.import-success{margin-top:12px;color:#176b59;font-weight:650}.share-import-dialog footer{display:flex;justify-content:flex-end;gap:9px;margin-top:20px}.import-primary,.import-secondary{min-height:42px;padding:0 15px;border:0;border-radius:12px;font-weight:650;cursor:pointer}.import-primary{background:#176b59;color:white}.import-secondary{background:#edf2ef;color:#25312c}.import-primary:disabled,.import-secondary:disabled{opacity:.55;cursor:not-allowed}@media(max-width:600px){.share-import-dialog{padding:20px;border-radius:20px}.share-import-dialog footer{flex-wrap:wrap}.share-import-dialog footer button{flex:1}}

/* Semantic surfaces keep text and controls legible in both JourneyIn themes. */
.share-import-dialog{border:1px solid var(--line);background:color-mix(in srgb,var(--md-surface-container-high) 94%,var(--surface));color:var(--ink)}
.share-import-dialog h2{color:var(--ink)}
.share-import-dialog header p{color:var(--muted)}
.import-eyebrow{color:var(--p)!important}
.import-source-form label,.import-item-copy strong,.import-source-summary strong{color:var(--ink)}
.import-source-form input{border-color:var(--line);color:var(--ink);background:var(--md-surface-container-low)}
.import-source-form input::placeholder{color:var(--muted);opacity:1}
.import-privacy{border:1px solid var(--line);color:var(--ink);background:color-mix(in srgb,var(--p) 8%,var(--md-surface-container-low))}
.import-source-summary small,.import-item-copy small{color:var(--muted)}
.import-item-list{border-color:var(--line);background:var(--md-surface-container-low)}
.import-item-row{border-color:var(--line);color:var(--ink)}
.import-item-copy .import-duplicate{color:var(--md-warning)}
.import-preview-box{border:1px solid color-mix(in srgb,var(--p) 28%,var(--line));color:var(--ink);background:color-mix(in srgb,var(--p) 9%,var(--md-surface-container-low))}
.import-preview-box p{color:var(--ink)}
.import-preview-box small{color:var(--muted)}
.import-error{color:var(--md-error)}
.import-success{color:var(--md-success)}
.import-primary{border:1px solid var(--p);color:var(--md-on-primary);background:var(--p)}
.import-secondary{border:1px solid var(--line);color:var(--ink);background:var(--md-surface-container-low)}
</style>
