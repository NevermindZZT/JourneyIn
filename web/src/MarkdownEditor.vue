<script setup lang="ts">
type MarkdownEditorMode = 'edit' | 'preview'

type MarkdownEditorProps = {
  modelValue: string
  previewHtml: string
  mode: MarkdownEditorMode
  placeholder?: string
  rows?: number
  editorLabel?: string
  previewLabel?: string
  editorAriaLabel?: string
}

const props = defineProps<MarkdownEditorProps>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'update:mode': [value: MarkdownEditorMode]
}>()

function updateValue(event: Event) {
  emit('update:modelValue', (event.target as HTMLTextAreaElement).value)
}
function setMode(mode: MarkdownEditorMode) {
  emit('update:mode', mode)
}
</script>

<template>
  <div class="markdown-editor" :class="'mode-' + props.mode">
    <div class="markdown-editor-switch" role="tablist" aria-label="Markdown 编辑和预览">
      <button type="button" role="tab" :aria-selected="props.mode === 'edit'" :class="{ selected: props.mode === 'edit' }" @click="setMode('edit')">编辑</button>
      <button type="button" role="tab" :aria-selected="props.mode === 'preview'" :class="{ selected: props.mode === 'preview' }" @click="setMode('preview')">预览</button>
    </div>
    <section class="markdown-editor-pane markdown-editor-input-pane">
      <div class="markdown-editor-pane-heading"><span>{{ props.editorLabel || 'MARKDOWN' }}</span><small>原始文本</small></div>
      <textarea :value="props.modelValue" :rows="props.rows || 5" :placeholder="props.placeholder" :aria-label="props.editorAriaLabel || 'Markdown 原始文本'" @input="updateValue"></textarea>
    </section>
    <section class="markdown-editor-pane markdown-editor-preview-pane">
      <div class="markdown-editor-pane-heading"><span>PREVIEW</span><small>{{ props.previewLabel || '渲染预览' }}</small></div>
      <div v-if="props.previewHtml" class="markdown markdown-editor-preview" v-html="props.previewHtml"></div>
      <p v-else class="muted markdown-preview-empty">输入 Markdown 后，这里会显示渲染结果。</p>
    </section>
  </div>
</template>
