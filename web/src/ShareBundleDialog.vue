<script setup lang="ts">
import { computed, ref, watch } from 'vue'

type TripOption = { id: string; title: string; start_date: string; end_date: string; can_edit?: boolean }
type ShareResponse = { id: string; url: string; expires_at?: string | null; permanent?: boolean; item_count: number; error?: { message?: string } }
type BundleSummary = { id: string; status: 'active' | 'expired' | 'revoked'; created_at: string; expires_at?: string; item_count: number; trip_titles: string[] }

const props = defineProps<{
  open: boolean
  trips: TripOption[]
  apiFetch: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
}>()
const emit = defineEmits<{ close: []; created: [] }>()

const selectedIDs = ref<string[]>([])
const search = ref('')
const expiry = ref<'7d' | '30d' | '90d' | 'permanent'>('30d')
const busy = ref(false)
const error = ref('')
const copied = ref(false)
const result = ref<ShareResponse | null>(null)
const bundles = ref<BundleSummary[]>([])
const loadingBundles = ref(false)
const visibleTrips = computed(() => props.trips.filter(trip => (trip.title + ' ' + trip.start_date).toLowerCase().includes(search.value.trim().toLowerCase())))
const selectedTrips = computed(() => props.trips.filter(trip => selectedIDs.value.includes(trip.id)))

watch(() => props.open, opened => {
  if (opened) { selectedIDs.value = []; search.value = ''; expiry.value = '30d'; error.value = ''; copied.value = false; result.value = null; void loadBundles() }
})

async function loadBundles() {
  loadingBundles.value = true
  try {
    const response = await props.apiFetch('/api/v1/share-bundles')
    const payload = await response.json() as { items?: BundleSummary[]; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '读取分享记录失败')
    bundles.value = payload.items || []
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '读取分享记录失败' }
  finally { loadingBundles.value = false }
}

function toggleTrip(id: string) {
  if (selectedIDs.value.includes(id)) { selectedIDs.value = selectedIDs.value.filter(value => value !== id); return }
  if (selectedIDs.value.length >= 50) { error.value = '一次最多分享 50 条行程'; return }
  error.value = ''
  selectedIDs.value = [...selectedIDs.value, id]
}
function dismiss() { if (!busy.value) emit('close') }
async function createBundle() {
  if (busy.value || selectedIDs.value.length === 0) return
  busy.value = true; error.value = ''; result.value = null
  try {
    const ttlSeconds = expiry.value === 'permanent' ? undefined : ({ '7d': 7, '30d': 30, '90d': 90 }[expiry.value] * 86400)
    const response = await props.apiFetch('/api/v1/share-bundles', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ trip_ids: selectedIDs.value, ttl_seconds: ttlSeconds, permanent: expiry.value === 'permanent' }) })
    const payload = await response.json() as ShareResponse
    if (!response.ok || !payload.url) throw new Error(payload.error?.message || '创建分享包失败')
    payload.url = new URL(payload.url, window.location.origin).toString()
    result.value = payload
    emit('created')
    void loadBundles()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '创建分享包失败' }
  finally { busy.value = false }
}
async function writeClipboard(text: string) {
  if (window.isSecureContext && navigator.clipboard?.writeText) {
    try { await navigator.clipboard.writeText(text); return } catch { /* use the legacy selection fallback */ }
  }
  const field = document.createElement('textarea')
  field.value = text
  field.readOnly = true
  field.setAttribute('aria-hidden', 'true')
  field.style.position = 'fixed'; field.style.top = '0'; field.style.left = '-9999px'; field.style.opacity = '0'
  document.body.appendChild(field)
  field.focus(); field.select(); field.setSelectionRange(0, field.value.length)
  let succeeded = false
  try { succeeded = document.execCommand('copy') } finally { field.remove() }
  if (!succeeded) throw new Error('浏览器未允许复制，请手动选择链接文本')
}
async function copyLink() {
  if (!result.value) return
  error.value = ''
  try { await writeClipboard(result.value.url); copied.value = true }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '无法复制链接，请手动选择链接文本' }
}
async function revokeBundle() {
  if (!result.value || !window.confirm('撤销此分享链接？之后将无法继续打开或导入，但已下载的副本不会被删除。')) return
  busy.value = true; error.value = ''
  try {
    const response = await props.apiFetch('/api/v1/share-bundles/' + encodeURIComponent(result.value.id) + '/revoke', { method: 'POST' })
    if (!response.ok) { const payload = await response.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '撤销分享失败') }
    result.value = null
    error.value = '分享链接已撤销'
    void loadBundles()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '撤销分享失败' }
  finally { busy.value = false }
}
async function revokeListedBundle(bundle: BundleSummary) {
  const title = bundle.trip_titles.join('、') || (bundle.item_count + ' 条行程')
  if (!window.confirm('撤销分享包“' + title + '”？之后不能再访问或导入，已导入副本不会删除。')) return
  busy.value = true; error.value = ''
  try {
    const response = await props.apiFetch('/api/v1/share-bundles/' + encodeURIComponent(bundle.id) + '/revoke', { method: 'POST' })
    if (!response.ok) { const payload = await response.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '撤销分享失败') }
    await loadBundles()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '撤销分享失败' }
  finally { busy.value = false }
}
</script>

<template>
  <div v-if="open" class="bundle-dialog-backdrop" @click.self="dismiss">
    <section class="bundle-dialog" role="dialog" aria-modal="true" aria-labelledby="share-bundle-title">
      <header><div><p class="bundle-eyebrow">CROSS-INSTANCE SHARING</p><h2 id="share-bundle-title">分享多个行程</h2><p>生成一个固定快照链接，可在其他 JourneyIn 实例中导入。</p></div><button class="modal-close" type="button" aria-label="关闭" @click="dismiss">×</button></header>
      <template v-if="!result">
        <label class="bundle-search">搜索行程<input v-model="search" type="search" placeholder="按标题或日期搜索" /></label>
        <div class="bundle-trip-list" role="group" aria-label="选择要分享的行程">
          <label v-for="trip in visibleTrips" :key="trip.id" class="bundle-trip-row">
            <input type="checkbox" :checked="selectedIDs.includes(trip.id)" @change="toggleTrip(trip.id)" />
            <span><strong>{{ trip.title }}</strong><small>{{ trip.start_date }} — {{ trip.end_date }}</small></span>
          </label>
          <p v-if="!visibleTrips.length" class="bundle-empty">没有匹配的行程。</p>
        </div>
        <div v-if="selectedTrips.length" class="bundle-selected"><span v-for="trip in selectedTrips" :key="trip.id" class="bundle-chip">{{ trip.title }} <button type="button" :aria-label="'移除 ' + trip.title" @click="toggleTrip(trip.id)">×</button></span></div>
        <fieldset class="bundle-expiry"><legend>链接有效期</legend><label><input v-model="expiry" type="radio" value="7d" />7 天</label><label><input v-model="expiry" type="radio" value="30d" />30 天（推荐）</label><label><input v-model="expiry" type="radio" value="90d" />90 天</label><label><input v-model="expiry" type="radio" value="permanent" />永久</label></fieldset>
        <p class="bundle-privacy">持有链接的人都可以查看并导入快照。撤销链接不会删除已下载或导入的行程。</p>
        <section class="bundle-history" aria-label="已创建的分享包">
          <div class="bundle-history-head"><strong>分享记录</strong><small>链接只在创建时显示；若链接遗失，请撤销后重新创建。</small></div>
          <p v-if="loadingBundles" class="bundle-history-empty">正在读取…</p>
          <p v-else-if="!bundles.length" class="bundle-history-empty">尚无其他分享记录。</p>
          <article v-for="bundle in bundles" :key="bundle.id" class="bundle-history-row">
            <span><strong>{{ bundle.trip_titles.join('、') || (bundle.item_count + ' 条行程') }}</strong><small>{{ bundle.status === 'active' ? '有效' : bundle.status === 'expired' ? '已过期' : '已撤销' }} · {{ bundle.expires_at ? '至 ' + new Date(bundle.expires_at).toLocaleString() : '永久' }}</small></span>
            <button v-if="bundle.status === 'active'" class="bundle-danger" type="button" :disabled="busy" @click="revokeListedBundle(bundle)">撤销</button>
          </article>
        </section>
        <p v-if="error" class="bundle-error" role="status">{{ error }}</p>
        <footer><button class="bundle-secondary" type="button" @click="dismiss">取消</button><button class="bundle-primary" type="button" :disabled="busy || !selectedIDs.length" @click="createBundle">{{ busy ? '正在创建…' : '创建分享链接（' + selectedIDs.length + '）' }}</button></footer>
      </template>
      <template v-else>
        <div class="bundle-result"><strong>分享链接已创建</strong><small>{{ result.item_count }} 条行程 · {{ result.permanent ? '永久有效' : (result.expires_at ? '有效期至 ' + new Date(result.expires_at).toLocaleString() : '有效期已设置') }}</small></div>
        <label class="bundle-url-label">分享链接<input readonly :value="result.url" @focus="($event.target as HTMLInputElement).select()" /></label>
        <p class="bundle-privacy">其他实例导入后会创建独立副本。撤销只阻止后续访问，不会远程删除已导入内容。</p>
        <p v-if="error" class="bundle-error" role="status">{{ error }}</p>
        <footer><button class="bundle-danger" type="button" :disabled="busy" @click="revokeBundle">撤销链接</button><button class="bundle-secondary" type="button" @click="dismiss">关闭</button><button class="bundle-primary" type="button" @click="copyLink">{{ copied ? '已复制 ✓' : '复制链接' }}</button></footer>
      </template>
    </section>
  </div>
</template>

<style scoped>
.bundle-dialog-backdrop{position:fixed;inset:0;z-index:1200;display:grid;place-items:center;padding:20px;background:#111a18a6}.bundle-dialog{position:relative;width:min(640px,100%);max-height:min(88vh,850px);overflow:auto;border-radius:24px;background:var(--surface,#fff);color:var(--on-surface,#19231f);padding:26px;box-shadow:0 24px 70px #0003}.bundle-dialog header{display:flex;justify-content:space-between;gap:16px}.bundle-dialog h2{margin:2px 0 6px;font-size:24px}.bundle-dialog header p{margin:0;color:var(--on-surface-variant,#68756f)}.bundle-eyebrow{font-size:11px;letter-spacing:.12em;font-weight:700;color:#357567!important}.bundle-close{border:0;background:transparent;font-size:26px;cursor:pointer;color:inherit;align-self:flex-start}.bundle-search,.bundle-url-label{display:grid;gap:7px;margin-top:18px;font-size:13px;font-weight:650}.bundle-search input,.bundle-url-label input{min-height:44px;border:1px solid #c8d2cd;border-radius:12px;padding:0 12px;font:inherit}.bundle-trip-list{max-height:240px;overflow:auto;margin-top:10px;border:1px solid #dce4e0;border-radius:14px}.bundle-trip-row{display:flex;align-items:center;gap:12px;padding:12px;border-bottom:1px solid #edf1ef;cursor:pointer}.bundle-trip-row:last-child{border-bottom:0}.bundle-trip-row input{flex:0 0 18px;width:18px;min-width:18px;height:18px;min-height:18px;accent-color:var(--p)}.bundle-expiry input{width:18px;height:18px;accent-color:var(--p)}.bundle-trip-row span{display:grid;min-width:0;flex:1;gap:3px}.bundle-trip-row small,.bundle-result small{color:#64716b}.bundle-empty{padding:12px;color:#66736e}.bundle-selected{display:flex;flex-wrap:wrap;gap:7px;margin-top:10px}.bundle-chip{display:inline-flex;align-items:center;gap:4px;padding:5px 10px;border-radius:99px;background:#e6f2ed;font-size:13px}.bundle-chip button{border:0;background:none;font-size:17px;cursor:pointer}.bundle-expiry{display:flex;flex-wrap:wrap;gap:12px 18px;margin:18px 0 0;padding:12px;border:1px solid #dce4e0;border-radius:14px}.bundle-expiry legend{padding:0 5px;font-size:13px;font-weight:650}.bundle-expiry label{display:flex;align-items:center;gap:6px;font-size:13px}.bundle-privacy{margin:15px 0 0;padding:12px 14px;border-radius:12px;background:#f4f7f5;color:#56655e;font-size:13px;line-height:1.5}.bundle-result{display:grid;gap:4px;margin:20px 0 12px}.bundle-error{color:#a22b2b;font-size:13px}footer{display:flex;justify-content:flex-end;gap:9px;margin-top:20px}.bundle-primary,.bundle-secondary,.bundle-danger{min-height:42px;padding:0 15px;border:0;border-radius:12px;font-weight:650;cursor:pointer}.bundle-primary{background:#176b59;color:white}.bundle-secondary{background:#edf2ef;color:#25312c}.bundle-danger{background:#fff0ed;color:#a33b32}.bundle-primary:disabled,.bundle-danger:disabled{opacity:.55;cursor:not-allowed}@media(max-width:600px){.bundle-dialog{padding:20px;border-radius:20px}.bundle-expiry{gap:10px}.bundle-dialog footer{flex-wrap:wrap}.bundle-dialog footer button{flex:1}}
.bundle-history{display:grid;gap:8px;margin-top:14px}.bundle-history-head{display:grid;gap:3px}.bundle-history-head small,.bundle-history-empty{margin:0;color:#68756f;font-size:12px}.bundle-history-row{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:10px 12px;border:1px solid #dce4e0;border-radius:12px}.bundle-history-row span{display:grid;gap:3px;min-width:0}.bundle-history-row strong{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.bundle-history-row small{color:#63716b;font-size:12px}

/* Use JourneyIn semantic Material 3 tokens in both light and dark themes. */
.bundle-dialog{border:1px solid var(--line);background:color-mix(in srgb,var(--md-surface-container-high) 94%,var(--surface));color:var(--ink)}
.bundle-dialog header p{color:var(--muted)}
.bundle-eyebrow{color:var(--p)!important}
.bundle-search,.bundle-url-label{color:var(--ink)}
.bundle-search input,.bundle-url-label input{border-color:var(--line);color:var(--ink);background:var(--md-surface-container-low)}
.bundle-search input::placeholder{color:var(--muted);opacity:1}
.bundle-trip-list{border-color:var(--line);background:var(--md-surface-container-low)}
.bundle-trip-row{border-color:var(--line);color:var(--ink)}
.bundle-trip-row small,.bundle-result small{color:var(--muted)}
.bundle-empty{color:var(--muted)}
.bundle-chip{border:1px solid var(--line);color:var(--ink);background:color-mix(in srgb,var(--p) 14%,var(--md-surface-container))}
.bundle-expiry{border-color:var(--line);color:var(--ink)}
.bundle-expiry label{color:var(--ink)}
.bundle-privacy{border:1px solid var(--line);color:var(--ink);background:color-mix(in srgb,var(--p) 8%,var(--md-surface-container-low))}
.bundle-history-row{background:var(--md-surface-container-low)}
.bundle-history-row small,.bundle-history-head small,.bundle-history-empty{color:var(--muted)}
.bundle-error{color:var(--md-error)}
.bundle-primary{border:1px solid var(--p);color:var(--md-on-primary);background:var(--p)}
.bundle-secondary{border:1px solid var(--line);color:var(--ink);background:var(--md-surface-container-low)}
.bundle-danger{border:1px solid color-mix(in srgb,var(--md-error) 45%,var(--line));color:var(--md-error);background:color-mix(in srgb,var(--md-error) 13%,var(--md-surface-container-low))}
.bundle-history-row>span{flex:1 1 0;min-width:0}.bundle-history-row>button.bundle-danger{flex:0 0 76px;width:76px;min-width:76px;min-height:42px;padding:0 12px;white-space:nowrap;word-break:keep-all}
</style>
