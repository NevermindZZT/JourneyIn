<script setup lang="ts">
const props = defineProps<{ open: boolean; canCreateTrips: boolean; canShareBundles: boolean }>()
const emit = defineEmits<{ close: []; importFile: []; importShare: []; shareBundle: [] }>()
function close() { emit('close') }
function choose(action: 'importFile' | 'importShare' | 'shareBundle') {
  if (action === 'importFile') emit('importFile')
  else if (action === 'importShare') emit('importShare')
  else emit('shareBundle')
  emit('close')
}
</script>

<template>
  <div v-if="open" class="transfer-backdrop" @click.self="close">
    <section class="transfer-dialog" role="dialog" aria-modal="true" aria-labelledby="transfer-title">
      <header class="transfer-header">
        <div><p class="transfer-eyebrow">JOURNEYIN DATA</p><h2 id="transfer-title">导入与分享</h2><p>在实例之间复制行程；分享内容为只读快照。</p></div>
        <button type="button" class="transfer-close" aria-label="关闭" @click="close">×</button>
      </header>
      <div class="transfer-options">
        <button v-if="canCreateTrips" type="button" class="transfer-option" @click="choose('importFile')">
          <span class="transfer-icon">↓</span><span><strong>导入 Trip JSON</strong><small>从本地 JSON 文件导入单条行程</small></span><b aria-hidden="true">›</b>
        </button>
        <button v-if="canCreateTrips" type="button" class="transfer-option" @click="choose('importShare')">
          <span class="transfer-icon">↗</span><span><strong>从分享导入</strong><small>粘贴实例链接，或选择分享包 JSON</small></span><b aria-hidden="true">›</b>
        </button>
        <button v-if="canShareBundles" type="button" class="transfer-option" @click="choose('shareBundle')">
          <span class="transfer-icon">□</span><span><strong>分享多个行程</strong><small>选择有编辑权限的行程，生成一条快照链接</small></span><b aria-hidden="true">›</b>
        </button>
      </div>
      <p class="transfer-note">跨实例导入会创建独立副本，不覆盖已有行程，也不会与来源持续同步。</p>
      <footer><button class="transfer-cancel" type="button" @click="close">取消</button></footer>
    </section>
  </div>
</template>

<style scoped>
.transfer-backdrop{position:fixed;inset:0;z-index:1190;display:grid;place-items:center;padding:20px;background:#050a0980}.transfer-dialog{width:min(520px,100%);padding:24px;border:1px solid var(--line);border-radius:24px;background:var(--md-surface-container-high);color:var(--ink);box-shadow:0 24px 70px #0005}.transfer-header{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:18px}.transfer-header h2{margin:3px 0 6px;font-size:24px;letter-spacing:-.03em;color:var(--ink)}.transfer-header p:last-child{margin:0;color:var(--muted);font-size:13px;line-height:1.5}.transfer-eyebrow{margin:0;color:var(--p);font-size:11px;font-weight:800;letter-spacing:.12em}.transfer-close{display:grid;flex:0 0 40px;width:40px;height:40px;place-items:center;border:1px solid var(--line);border-radius:12px;background:var(--md-surface-container-low);color:var(--ink);font-size:24px}.transfer-options{display:grid;gap:9px}.transfer-option{display:flex;width:100%;min-height:70px;align-items:center;gap:13px;padding:12px 14px;border:1px solid var(--line);border-radius:15px;background:var(--md-surface-container-low);color:var(--ink);text-align:left}.transfer-option:focus-visible{outline:3px solid color-mix(in srgb,var(--p) 55%,transparent);outline-offset:2px}.transfer-option:hover{border-color:var(--p);background:color-mix(in srgb,var(--p) 10%,var(--md-surface-container-low))}.transfer-icon{display:grid;flex:0 0 42px;width:42px;height:42px;place-items:center;border-radius:13px;background:color-mix(in srgb,var(--p) 16%,var(--md-surface-container));color:var(--p);font-size:22px;font-weight:800}.transfer-option>span:nth-child(2){display:grid;min-width:0;flex:1;gap:4px}.transfer-option strong{font-size:15px;color:var(--ink)}.transfer-option small{color:var(--muted);font-size:12px;line-height:1.4}.transfer-option b{color:var(--muted);font-size:23px}.transfer-note{margin:14px 0 0;padding:12px 14px;border:1px solid var(--line);border-radius:13px;background:color-mix(in srgb,var(--p) 7%,var(--md-surface-container-low));color:var(--muted);font-size:12px;line-height:1.5}.transfer-dialog footer{display:flex;justify-content:flex-end;margin-top:16px}.transfer-cancel{min-height:44px;padding:0 18px;border:1px solid var(--line);border-radius:12px;background:var(--md-surface-container-low);color:var(--ink);font-weight:700}@media(max-width:600px){.transfer-dialog{padding:20px;border-radius:20px}.transfer-option{min-height:66px}.transfer-dialog footer .transfer-cancel{width:100%}}
</style>
