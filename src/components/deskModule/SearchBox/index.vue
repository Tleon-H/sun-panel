<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NCheckbox } from 'naive-ui'
import { decideSearchConfirm, decideSearchKey } from '@/utils/search/filterIcons'
import { buildSearchUrl, addCustomEngine, currentSearchEngine, moveEngine, normalizeSearchConfig, removeCustomEngine, selectEngine, toPersistedSearchConfig, updateCustomEngine, type SearchConfig, type SearchConfigError, type SearchEngineRecord } from '@/utils/search/engines'
import { SvgIcon } from '@/components/common'
import { useModuleConfig } from '@/store/modules'
import { useAuthStore } from '@/store'
import { VisitMode } from '@/enums/auth'
import { t } from '@/locales'

import SvgSrcBaidu from '@/assets/search_engine_svg/baidu.svg'
import SvgSrcBing from '@/assets/search_engine_svg/bing.svg'
import SvgSrcGoogle from '@/assets/search_engine_svg/google.svg'

const props = withDefaults(defineProps<{
  background?: string
  textColor?: string
  localSearchEnabled?: boolean
  matchCount?: number
}>(), {
  background: '#2a2a2a6b',
  textColor: 'white',
  localSearchEnabled: false,
  matchCount: 0,
})

const emits = defineEmits(['itemSearch', 'moveMatch', 'openLocal'])

const builtinIcons: Record<string, string> = {
  google: SvgSrcGoogle,
  baidu: SvgSrcBaidu,
  bing: SvgSrcBing,
}

const moduleConfigName = 'deskModuleSearchBox'
const moduleConfig = useModuleConfig()
const authStore = useAuthStore()
const searchTerm = ref('')
const isFocused = ref(false)
const searchSelectListShow = ref(false)
const config = ref<SearchConfig>(normalizeSearchConfig(null))
const draftTitle = ref('')
const draftUrl = ref('')
const editingId = ref<string | null>(null)
const formError = ref<SearchConfigError | ''>('')

const canEdit = computed(() => authStore.visitMode !== VisitMode.VISIT_MODE_PUBLIC)
const current = computed(() => currentSearchEngine(config.value))

const onFocus = (): void => {
  isFocused.value = true
}

const onBlur = (): void => {
  isFocused.value = false
}

function engineIcon(engine: SearchEngineRecord): string {
  return engine.builtin ? builtinIcons[engine.id] || '' : ''
}

function engineLetter(engine: SearchEngineRecord): string {
  return Array.from(engine.title.trim() || '?')[0]
}

function handleEngineClick() {
  if (!canEdit.value)
    return
  searchSelectListShow.value = !searchSelectListShow.value
}

async function persist(next: SearchConfig) {
  if (!canEdit.value)
    return
  config.value = next
  await moduleConfig.saveToCloud(moduleConfigName, toPersistedSearchConfig(next))
}

function handleEngineUpdate(engine: SearchEngineRecord) {
  void persist(selectEngine(config.value, engine.id))
  searchSelectListShow.value = false
}

function handleSearchClick() {
  const fullUrl = buildSearchUrl(current.value.url, searchTerm.value)
  if (!fullUrl)
    return
  handleClearSearchTerm()
  if (config.value.newWindowOpen)
    window.open(fullUrl)
  else
    window.location.href = fullUrl
}

function handleConfirm() {
  const decision = decideSearchConfirm({
    keyword: searchTerm.value,
    localSearchEnabled: props.localSearchEnabled,
    matchCount: props.matchCount,
  })
  if (decision === 'open-local')
    emits('openLocal')
  else
    handleSearchClick()
}

function handleKeydown(event: KeyboardEvent) {
  const decision = decideSearchKey({
    key: event.key,
    isComposing: event.isComposing,
    keyCode: event.keyCode,
    localSearchEnabled: props.localSearchEnabled,
    matchCount: props.matchCount,
  })
  if (decision.type === 'ignore')
    return
  event.preventDefault()
  if (decision.type === 'move')
    emits('moveMatch', decision.delta)
  else if (decision.type === 'confirm')
    handleConfirm()
  else
    handleClearSearchTerm()
}

function handleItemSearch(event?: Event) {
  const value = event?.target instanceof HTMLInputElement ? event.target.value : searchTerm.value
  searchTerm.value = value
  emits('itemSearch', value)
}

function handleClearSearchTerm() {
  searchTerm.value = ''
  emits('itemSearch', searchTerm.value)
}

function formMessage(error: SearchConfigError | ''): string {
  if (error === 'title')
    return t('deskModule.searchBox.engineErrorTitle')
  if (error === 'protocol')
    return t('deskModule.searchBox.engineErrorProtocol')
  if (error === 'duplicate' || error === 'limit')
    return t('deskModule.searchBox.engineErrorDuplicate')
  if (error === 'builtin')
    return t('deskModule.searchBox.engineErrorBuiltin')
  return ''
}

function beginEdit(engine: SearchEngineRecord) {
  if (engine.builtin)
    return
  editingId.value = engine.id
  draftTitle.value = engine.title
  draftUrl.value = engine.url
  formError.value = ''
}

function cancelEdit() {
  editingId.value = null
  draftTitle.value = ''
  draftUrl.value = ''
  formError.value = ''
}

function submitDraft() {
  const result = editingId.value
    ? updateCustomEngine(config.value, editingId.value, draftTitle.value, draftUrl.value)
    : addCustomEngine(config.value, draftTitle.value, draftUrl.value)
  if (!result.ok) {
    formError.value = result.error
    return
  }
  cancelEdit()
  void persist(result.config)
}

function moveAt(index: number, delta: number) {
  void persist(moveEngine(config.value, index, delta))
}

function removeAt(engine: SearchEngineRecord) {
  const result = removeCustomEngine(config.value, engine.id)
  if (!result.ok) {
    formError.value = result.error
    return
  }
  if (editingId.value === engine.id)
    cancelEdit()
  void persist(result.config)
}

function onNewWindow(checked: boolean) {
  void persist({ ...config.value, newWindowOpen: checked })
}

onMounted(() => {
  moduleConfig.getValueByNameFromCloud<unknown>(moduleConfigName).then(({ code, data }) => {
    config.value = normalizeSearchConfig(code === 0 ? data : null)
  }).catch(() => {
    config.value = normalizeSearchConfig(null)
  })
})
</script>

<template>
  <div class="search-box w-full">
    <div class="search-container flex rounded-2xl items-center justify-center text-white w-full" :style="{ background, color: textColor }" :class="{ focused: isFocused }">
      <div class="search-box-btn-engine w-[40px] flex justify-center cursor-pointer" @click="handleEngineClick">
        <img v-if="engineIcon(current)" class="engine-mark" :src="engineIcon(current)" alt="">
        <span v-else class="engine-letter">{{ engineLetter(current) }}</span>
      </div>

      <input v-model="searchTerm" :placeholder="$t('deskModule.searchBox.inputPlaceholder')" :aria-keyshortcuts="localSearchEnabled ? 'ArrowUp ArrowDown Enter Escape' : 'Enter Escape'" @focus="onFocus" @blur="onBlur" @input="handleItemSearch" @keydown="handleKeydown">

      <div v-if="searchTerm !== ''" class="search-box-btn-clear w-[25px] mr-[10px] flex justify-center cursor-pointer" @click="handleClearSearchTerm">
        <SvgIcon style="width: 20px;height: 20px;" icon="line-md:close-small" />
      </div>
      <div class="search-box-btn-search w-[25px] flex justify-center cursor-pointer" @click="handleSearchClick">
        <SvgIcon style="width: 20px;height: 20px;" icon="iconamoon:search-fill" />
      </div>
    </div>

    <div v-if="searchSelectListShow" class="w-full mt-[10px] rounded-xl p-[10px]" :style="{ background, color: textColor }">
      <div
        v-for="(item, index) in config.engines"
        :key="item.id"
        class="engine-row"
      >
        <button type="button" class="engine-pick" :title="item.title" @click="handleEngineUpdate(item)">
          <img v-if="engineIcon(item)" class="engine-mark" :src="engineIcon(item)" alt="">
          <span v-else class="engine-letter">{{ engineLetter(item) }}</span>
          <span class="engine-title">{{ item.title }}</span>
        </button>
        <button type="button" class="engine-action" :disabled="index === 0" @click="moveAt(index, -1)">
          {{ $t('deskModule.searchBox.engineUp') }}
        </button>
        <button type="button" class="engine-action" :disabled="index === config.engines.length - 1" @click="moveAt(index, 1)">
          {{ $t('deskModule.searchBox.engineDown') }}
        </button>
        <button v-if="!item.builtin" type="button" class="engine-action" @click="beginEdit(item)">
          {{ $t('deskModule.searchBox.engineEdit') }}
        </button>
        <button v-if="!item.builtin" type="button" class="engine-action" @click="removeAt(item)">
          {{ $t('deskModule.searchBox.engineDelete') }}
        </button>
      </div>

      <div class="engine-form">
        <input v-model="draftTitle" :placeholder="$t('deskModule.searchBox.engineTitle')">
        <input v-model="draftUrl" placeholder="https://example.com/search?q=%s" spellcheck="false">
        <div class="engine-form-actions">
          <button type="button" class="engine-action" @click="submitDraft">
            {{ editingId ? $t('deskModule.searchBox.engineSave') : $t('deskModule.searchBox.engineAdd') }}
          </button>
          <button v-if="editingId" type="button" class="engine-action" @click="cancelEdit">
            {{ $t('deskModule.searchBox.engineCancel') }}
          </button>
        </div>
        <p v-if="formError" class="engine-error">
          {{ formMessage(formError) }}
        </p>
        <p class="engine-help">
          {{ $t('deskModule.searchBox.engineHelp') }}
        </p>
      </div>

      <div class="mt-[10px]">
        <NCheckbox :checked="config.newWindowOpen" @update:checked="onNewWindow">
          <span :style="{ color: textColor }">
            {{ $t('deskModule.searchBox.openWithNewOpen') }}
          </span>
        </NCheckbox>
      </div>
    </div>
    <p v-if="isFocused && localSearchEnabled" class="leon-search-hint">
      {{ $t('deskModule.searchBox.keyboardHint') }}
    </p>
  </div>
</template>

<style scoped>
.search-container {
  border: 1px solid #ccc;
  transition: box-shadow 0.5s,backdrop-filter 0.5s;
  padding: 2px 10px;
  backdrop-filter:blur(2px);
  flex-wrap: nowrap;
}

.focused, .search-container:hover {
  box-shadow: 0px 0px 30px -5px rgba(41, 41, 41, 0.45);
  -webkit-box-shadow: 0px 0px 30px -5px rgba(0, 0, 0, 0.45);
  -moz-box-shadow: 0px 0px 30px -5px rgba(0, 0, 0, 0.45);
  backdrop-filter:blur(5px)
}

input {
  background-color: transparent;
  box-sizing: border-box;
  width: auto;
  flex: 1;
  min-width: 0;
  height: 40px;
  padding: 10px 5px;
  border: none;
  outline: none;
  font-size: 17px;
  color: inherit;
}

.engine-row,
.engine-form-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.engine-row {
  margin-bottom: 8px;
}

.engine-pick {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.engine-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.engine-mark,
.engine-letter {
  width: 20px;
  height: 20px;
  flex: 0 0 20px;
}

.engine-letter {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.9);
  color: #0f172a;
  font-size: 12px;
}

.engine-action {
  border: 1px solid rgba(255, 255, 255, 0.45);
  background: transparent;
  color: inherit;
  border-radius: 8px;
  padding: 2px 6px;
  cursor: pointer;
}

.engine-action:disabled {
  opacity: 0.4;
  cursor: default;
}

.engine-form input {
  display: block;
  width: 100%;
  margin-top: 6px;
  border: 1px solid rgba(255, 255, 255, 0.45);
  border-radius: 8px;
}

.engine-help,
.engine-error {
  margin: 8px 0 0;
  font-size: 13px;
  line-height: 1.4;
}

.engine-error {
  color: #fecaca;
}
</style>
