<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { IonIcon } from '@ionic/vue'
import { chevronDownOutline } from 'ionicons/icons'

type SelectOption = { value: string; label: string; description?: string; disabled?: boolean }

const props = withDefaults(defineProps<{
  modelValue: string
  options: SelectOption[]
  ariaLabel?: string
  placeholder?: string
  disabled?: boolean
}>(), {
  ariaLabel: '选择',
  placeholder: '请选择',
  disabled: false,
})

const emit = defineEmits<{ (event: 'update:modelValue', value: string): void }>()
const root = ref<HTMLElement | null>(null)
const open = ref(false)
const activeIndex = ref(-1)
const listboxID = 'journeyin-select-' + Math.random().toString(36).slice(2)

const selectedOption = computed(() => props.options.find(option => option.value === props.modelValue) || null)

function openMenu() {
  if (props.disabled) return
  open.value = true
  activeIndex.value = Math.max(0, props.options.findIndex(option => option.value === props.modelValue))
  void nextTick(() => root.value?.querySelector<HTMLElement>('.ui-select-option.active')?.scrollIntoView({ block: 'nearest' }))
}

function closeMenu() {
  open.value = false
  activeIndex.value = -1
}

function toggleMenu() {
  if (open.value) closeMenu()
  else openMenu()
}

function choose(option: SelectOption) {
  if (option.disabled) return
  emit('update:modelValue', option.value)
  closeMenu()
}

function moveActive(direction: 1 | -1) {
  const available = props.options.map((option, index) => ({ option, index })).filter(item => !item.option.disabled)
  if (!available.length) return
  const current = Math.max(0, available.findIndex(item => item.index === activeIndex.value))
  const next = (current + direction + available.length) % available.length
  activeIndex.value = available[next].index
  void nextTick(() => root.value?.querySelector<HTMLElement>('.ui-select-option.active')?.scrollIntoView({ block: 'nearest' }))
}

function handleDocumentPointerDown(event: PointerEvent) {
  if (!root.value?.contains(event.target as Node)) closeMenu()
}

function handleKeydown(event: KeyboardEvent) {
  if (!open.value) {
    if (event.target !== root.value?.querySelector('.ui-select-trigger')) return
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp' || event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      openMenu()
    }
    return
  }
  if (event.key === 'Escape') { event.preventDefault(); closeMenu(); return }
  if (event.key === 'ArrowDown') { event.preventDefault(); moveActive(1); return }
  if (event.key === 'ArrowUp') { event.preventDefault(); moveActive(-1); return }
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    const option = props.options[activeIndex.value]
    if (option) choose(option)
  }
}

watch(() => props.modelValue, () => {
  if (open.value) activeIndex.value = props.options.findIndex(option => option.value === props.modelValue)
})

onMounted(() => {
  document.addEventListener('pointerdown', handleDocumentPointerDown)
  document.addEventListener('keydown', handleKeydown)
})
onUnmounted(() => {
  document.removeEventListener('pointerdown', handleDocumentPointerDown)
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <div ref="root" class="ui-select" :class="{ open, disabled }">
    <button
      class="ui-select-trigger"
      type="button"
      role="combobox"
      :aria-label="ariaLabel"
      :aria-expanded="open"
      :aria-controls="listboxID"
      :disabled="disabled"
      @click="toggleMenu"
    >
      <span class="ui-select-value">{{ selectedOption?.label || placeholder }}</span>
      <IonIcon class="ui-select-chevron" :icon="chevronDownOutline" aria-hidden="true" />
    </button>
    <div v-if="open" :id="listboxID" class="ui-select-menu" role="listbox" :aria-label="ariaLabel">
      <button
        v-for="(option, index) in options"
        :key="option.value"
        class="ui-select-option"
        :class="{ active: index === activeIndex, selected: option.value === modelValue }"
        type="button"
        role="option"
        :aria-selected="option.value === modelValue"
        :disabled="option.disabled"
        @click="choose(option)"
      >
        <span><strong>{{ option.label }}</strong><small v-if="option.description">{{ option.description }}</small></span>
        <span v-if="option.value === modelValue" class="ui-select-check" aria-hidden="true">✓</span>
      </button>
    </div>
  </div>
</template>
