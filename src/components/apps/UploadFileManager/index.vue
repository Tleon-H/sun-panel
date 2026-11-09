<script setup lang="ts">
import { NAlert, NButton, NButtonGroup, NCard, NEllipsis, NGrid, NGridItem, NImage, NImageGroup, NInput, NPagination, NSelect, NSpin, useDialog, useMessage } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'
import { deletes, getList, setPublic, updateCategory, uploadFiles } from '@/api/system/file'
import { set as savePanelConfig } from '@/api/panel/userConfig'
import { RoundCardModal, SvgIcon } from '@/components/common'
import { copyToClipboard, timeFormat } from '@/utils/cmn'
import { displayFileCategory, fileCategories, ownFileQuery, pageAfterListChange, uploadCategoryForEntry, type FileCategory } from '@/utils/file/category'
import { t } from '@/locales'
import { useAuthStore, usePanelState } from '@/store'

interface InfoModalState {
  title: string
  show: boolean
  fileInfo: File.Info | null
}
const imageList = ref<File.Info[]>([])
const ms = useMessage()
const dialog = useDialog()
const panelStore = usePanelState()
const authStore = useAuthStore()
const isAdmin = computed(() => authStore.userInfo?.role === 1)
const loading = ref(false)
const page = ref(1)
const total = ref(0)
const keyword = ref('')
const category = ref<'' | FileCategory>('')
const infoModalState = ref<InfoModalState>({
  show: false,
  title: '',
  fileInfo: null,
})
const batchInput = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const batchStatus = ref('')
let keywordTimer: ReturnType<typeof setTimeout> | null = null
let listSerial = 0
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / 12)))
const categoryOptions = computed(() => fileCategories.map(value => ({
  label: t(`apps.uploadsFileManager.category.${value}`),
  value,
})))
const filterOptions = computed(() => [
  { label: t('apps.uploadsFileManager.category.all'), value: '' as const },
  ...categoryOptions.value,
])

async function getFileList() {
  const serial = ++listSerial
  loading.value = true
  try {
    const query = ownFileQuery({
      page: page.value,
      category: category.value,
      keyword: keyword.value,
    })
    const { data } = await getList<Common.ListResponse<File.Info[]>>(query)
    if (serial !== listSerial)
      return
    imageList.value = (data.list ?? []).map(item => ({
      ...item,
      category: displayFileCategory(item.category),
    }))
    total.value = data.count ?? 0
    const nextPage = pageAfterListChange(page.value, total.value, query.limit)
    if (nextPage !== page.value) {
      page.value = nextPage
      await getFileList()
      return
    }
  }
  finally {
    if (serial === listSerial)
      loading.value = false
  }
}

function selectCategory(value: '' | FileCategory) {
  category.value = value
  page.value = 1
  void getFileList()
}

function onKeywordInput() {
  page.value = 1
  if (keywordTimer)
    clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => {
    void getFileList()
  }, 250)
}

function changePage(next: number) {
  page.value = next
  void getFileList()
}

function openBatchPicker() {
  batchInput.value?.click()
}

async function onBatchSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  input.value = ''
  if (files.length === 0)
    return
  uploading.value = true
  batchStatus.value = ''
  try {
    const { code, data, msg } = await uploadFiles(files, uploadCategoryForEntry('other'))
    if (code !== 0) {
      batchStatus.value = `${t('common.uploadFail')}${msg ? `:${msg}` : ''}`
      ms.error(batchStatus.value)
      return
    }
    const saved = data?.saved?.length ?? 0
    const failed = data?.errFiles ?? []
    const parts = [t('apps.uploadsFileManager.batchSaved', { count: saved })]
    if (failed.length > 0)
      parts.push(t('apps.uploadsFileManager.batchFailed', { names: failed.join('、') }))
    batchStatus.value = parts.join('')
    if (saved > 0)
      ms.success(t('apps.uploadsFileManager.batchSaved', { count: saved }))
    else
      ms.error(t('common.uploadFail'))
    if (failed.length > 0)
      ms.warning(t('apps.uploadsFileManager.batchFailed', { names: failed.join('、') }))
    category.value = 'uncategorized'
    page.value = 1
    await getFileList()
  }
  finally {
    uploading.value = false
  }
}

async function togglePublic(item: File.Info) {
  if (!isAdmin.value || !item.id)
    return
  const { code, msg } = await setPublic(item.id, !item.public)
  if (code === 0) {
    ms.success(t('common.success'))
    await getFileList()
  }
  else {
    ms.error(`${t('common.failed')}:${msg}`)
  }
}

async function changeCategory(item: File.Info, value: string) {
  const next = displayFileCategory(value)
  if (!item.id || next === item.category)
    return
  const { code, msg } = await updateCategory(item.id, next)
  if (code === 0) {
    ms.success(t('common.success'))
    await getFileList()
  }
  else {
    ms.error(`${t('common.failed')}:${msg}`)
  }
}

async function copyImageUrl(text: string) {
  const res = await copyToClipboard(text)
  if (res)
    ms.success(t('apps.uploadsFileManager.copySuccess'))

  else
    ms.error(t('apps.uploadsFileManager.copyFailed'))
}

function handleDelete(id: number) {
  dialog.warning({
    title: t('common.warning'),
    content: t('apps.uploadsFileManager.deleteWarningText'),
    positiveText: t('common.confirm'),
    negativeText: t('common.cancel'),
    onPositiveClick: () => {
      deletesImges(id)
    },
  })
}

async function deletesImges(id: number) {
  try {
    const { code, msg } = await deletes([id])
    if (code === 0) {
      getFileList()
      ms.success(t('common.success'))
    }
    else {
      ms.error(`${t('common.failed')}:${msg}`)
    }
  }
  catch (error) {
    ms.error(t('common.failed'))
  }
}

function handleInfoClick(fileInfo: File.Info) {
  infoModalState.value.fileInfo = fileInfo
  infoModalState.value.show = true
}

function handleSetWallpaper(imgSrc: string) {
  panelStore.panelConfig.backgroundImageSrc = imgSrc
  savePanelConfig({ panel: panelStore.panelConfig })
}

onMounted(() => {
  getFileList()
})
</script>

<template>
  <div class="bg-slate-200 dark:bg-zinc-900 p-2 h-full">
    <NSpin v-show="loading" size="small" />
    <NAlert type="info" :bordered="false">
      {{ $t('apps.uploadsFileManager.alertText') }}
      <div v-if="isAdmin">
        {{ $t('apps.publicGallery.manageHint') }}
      </div>
    </NAlert>
    <div class="batch-upload">
      <input
        ref="batchInput"
        class="batch-file"
        type="file"
        multiple
        accept=".png,.jpg,.jpeg,.gif,.webp,.svg,.ico,image/png,image/jpeg,image/gif,image/webp,image/svg+xml,image/x-icon"
        data-testid="batch-upload-input"
        @change="onBatchSelected"
      >
      <NButton size="small" data-testid="batch-upload" :loading="uploading" @click="openBatchPicker">
        {{ $t('apps.uploadsFileManager.batchUpload') }}
      </NButton>
      <div class="text-xs mt-1">
        {{ $t('apps.uploadsFileManager.batchHint') }}
      </div>
      <div v-if="batchStatus" class="text-xs mt-1" data-testid="batch-upload-result">
        {{ batchStatus }}
      </div>
    </div>
    <div class="file-filters">
      <button
        v-for="item in filterOptions"
        :key="item.value || 'all'"
        type="button"
        class="file-filter"
        :class="{ active: category === item.value }"
        @click="selectCategory(item.value)"
      >
        {{ item.label }}
      </button>
    </div>
    <NInput
      v-model:value="keyword"
      class="mt-2"
      clearable
      data-testid="file-name-search"
      :placeholder="$t('apps.uploadsFileManager.searchPlaceholder')"
      @update:value="onKeywordInput"
    />
    <div class="flex justify-center mt-2">
      <div v-if="imageList.length === 0 && !loading" class="flex">
        {{ keyword || category ? $t('apps.uploadsFileManager.noMatch') : $t('apps.uploadsFileManager.nothingText') }}
      </div>
      <div v-else data-testid="own-file-list">
        <NImageGroup>
        <NGrid cols="2 300:2 600:4 900:6 1100:9" :x-gap="5" :y-gap="5">
          <NGridItem v-for=" item, index in imageList" :key="item.id || index">
            <div class="file-card" :data-filename="item.fileName" :data-public="item.public ? 'true' : 'false'">
            <NCard size="small" style="border-radius: 5px;" :bordered="true">
              <template #cover>
                <div class="card transparent-grid">
                  <NImage :lazy="true" style="object-fit: contain;height: 100%;" :src="item.src" />
                </div>
              </template>
              <template #footer>
                <span class="text-xs">
                  <NEllipsis>
                    {{ item.fileName }}
                  </NEllipsis>
                </span>
                <div class="mt-1">
                  <NSelect
                    size="tiny"
                    :value="displayFileCategory(item.category)"
                    :options="categoryOptions"
                    @update:value="(value: string) => changeCategory(item, value)"
                  />
                </div>
                <div v-if="item.public" class="text-xs mt-1">
                  {{ $t('apps.publicGallery.published') }}
                </div>
                <div class="flex justify-center mt-[10px]">
                  <NButtonGroup>
                    <NButton size="tiny" tertiary style="cursor: pointer;" :title="$t('apps.uploadsFileManager.copyLink')" @click="copyImageUrl(item.src)">
                      <template #icon>
                        <SvgIcon icon="ion-copy" />
                      </template>
                    </NButton>
                    <NButton size="tiny" tertiary style="cursor: pointer;" :title="timeFormat(item.createTime)" @click="handleInfoClick(item)">
                      <template #icon>
                        <SvgIcon icon="mdi-information-box-outline" />
                      </template>
                    </NButton>
                    <NButton size="tiny" tertiary style="cursor: pointer;" :title="$t('apps.uploadsFileManager.setWallpaper')" @click="handleSetWallpaper(item.src)">
                      <template #icon>
                        <SvgIcon icon="lucide:wallpaper" />
                      </template>
                    </NButton>
                    <NButton
                      v-if="isAdmin"
                      size="tiny"
                      tertiary
                      data-testid="file-public-toggle"
                      @click="togglePublic(item)"
                    >
                      {{ item.public ? $t('apps.publicGallery.unpublish') : $t('apps.publicGallery.publish') }}
                    </NButton>
                    <NButton size="tiny" tertiary type="error" style="cursor: pointer;" :title="$t('common.delete')" @click="handleDelete(item.id as number)">
                      <template #icon>
                        <SvgIcon icon="material-symbols-delete" />
                      </template>
                    </NButton>
                  </NButtonGroup>
                </div>
              </template>
            </NCard>
            </div>
          </NGridItem>
        </NGrid>
        </NImageGroup>
      </div>
    </div>
    <div v-if="total > 12" class="file-pages">
      <NPagination :page="page" :page-count="pageCount" @update:page="changePage" />
    </div>

    <RoundCardModal v-model:show="infoModalState.show" style="max-width: 300px;" size="small" :title="$t('apps.uploadsFileManager.infoTitle')">
      <div>
        <div>
          <div class="mb-2">
            <span class="text-slate-500">
              {{ $t('apps.uploadsFileManager.fileName') }}
            </span>
            <div class="text-xs">
              {{ infoModalState.fileInfo?.fileName }}
            </div>
          </div>
          <div class="mb-2">
            <span class="text-slate-500">
              {{ $t('apps.uploadsFileManager.path') }}
            </span>
            <div class="text-xs">
              {{ infoModalState.fileInfo?.src }}
            </div>
          </div>
          <div class="mb-2">
            <span class="text-slate-500">
              {{ $t('apps.uploadsFileManager.uploadTime') }}
            </span>
            <div class="text-xs">
              {{ timeFormat(infoModalState.fileInfo?.createTime) }}
            </div>
          </div>
          <div class="mb-2">
            <span class="text-slate-500">
              {{ $t('apps.uploadsFileManager.categoryLabel') }}
            </span>
            <div class="text-xs">
              {{ $t(`apps.uploadsFileManager.category.${displayFileCategory(infoModalState.fileInfo?.category)}`) }}
            </div>
          </div>
        </div>
      </div>
    </RoundCardModal>
  </div>
</template>

<style scoped>
.card {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 80px;
}

.transparent-grid {
  background-image: linear-gradient(45deg, #f0f0f0 25%, transparent 25%, transparent 75%, #f0f0f0 75%),
    linear-gradient(45deg, #f0f0f0 25%, transparent 25%, transparent 75%, #f0f0f0 75%);
  background-size: 16px 16px;
  background-position: 0 0, 8px 8px;
}

.file-filters,
.file-pages {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

.file-pages {
  justify-content: center;
}

.file-filter {
  border: 1px solid rgba(148, 163, 184, 0.8);
  background: transparent;
  border-radius: 999px;
  padding: 4px 10px;
  cursor: pointer;
}

.file-filter.active {
  background: #0f172a;
  color: #f8fafc;
  border-color: #e2e8f0;
}

.batch-upload {
  position: relative;
  margin-top: 8px;
}

.batch-file {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
}
</style>
