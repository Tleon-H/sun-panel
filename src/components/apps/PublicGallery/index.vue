<script setup lang="ts">
import { NAlert, NButton, NButtonGroup, NCard, NEllipsis, NGrid, NGridItem, NImage, NImageGroup, NInput, NPagination, useMessage } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'
import { getPublicList } from '@/api/system/file'
import { set as savePanelConfig } from '@/api/panel/userConfig'
import { SvgIcon } from '@/components/common'
import { copyToClipboard } from '@/utils/cmn'
import { displayFileCategory, fileCategories, pageAfterListChange, publicFileQuery, type FileCategory } from '@/utils/file/category'
import { t } from '@/locales'
import { usePanelState } from '@/store'

const props = withDefaults(defineProps<{
  lockedCategory?: '' | FileCategory
  selectable?: boolean
}>(), {
  lockedCategory: '',
  selectable: false,
})

const emit = defineEmits<{
  (e: 'select', src: string): void
}>()

const images = ref<File.Info[]>([])
const loading = ref(false)
const page = ref(1)
const total = ref(0)
const keyword = ref('')
const category = ref<'' | FileCategory>(props.lockedCategory || '')
const ms = useMessage()
const panelStore = usePanelState()
let keywordTimer: ReturnType<typeof setTimeout> | null = null
let listSerial = 0
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / 12)))

const categoryOptions = computed(() => [
  { label: t('apps.uploadsFileManager.category.all'), value: '' as const },
  ...fileCategories.map(value => ({
    label: t(`apps.uploadsFileManager.category.${value}`),
    value,
  })),
])

async function load() {
  const serial = ++listSerial
  loading.value = true
  try {
    const query = publicFileQuery({
      page: page.value,
      category: props.lockedCategory || category.value,
      keyword: keyword.value,
    })
    const { data } = await getPublicList<Common.ListResponse<File.Info[]>>(query)
    if (serial !== listSerial)
      return
    images.value = (data.list ?? []).map(item => ({
      ...item,
      category: displayFileCategory(item.category),
      public: true,
    }))
    total.value = data.count ?? 0
    const nextPage = pageAfterListChange(page.value, total.value, query.limit)
    if (nextPage !== page.value) {
      page.value = nextPage
      await load()
    }
  }
  finally {
    if (serial === listSerial)
      loading.value = false
  }
}

function changePage(next: number) {
  page.value = next
  void load()
}

function selectCategory(value: '' | FileCategory) {
  category.value = value
  page.value = 1
  void load()
}

function onKeywordInput() {
  page.value = 1
  if (keywordTimer)
    clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => {
    void load()
  }, 250)
}

async function copyImageUrl(text: string) {
  const copied = await copyToClipboard(text)
  if (copied)
    ms.success(t('apps.uploadsFileManager.copySuccess'))
  else
    ms.error(t('apps.uploadsFileManager.copyFailed'))
}

function useImage(src: string) {
  emit('select', src)
}

function setWallpaper(src: string) {
  panelStore.panelConfig.backgroundImageSrc = src
  void savePanelConfig({ panel: panelStore.panelConfig })
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="public-gallery" data-testid="public-gallery">
    <NAlert type="info" :bordered="false">
      {{ $t('apps.publicGallery.alert') }}
    </NAlert>
    <div v-if="!lockedCategory" class="public-filters">
      <button
        v-for="item in categoryOptions"
        :key="item.value || 'all'"
        type="button"
        class="public-filter"
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
      data-testid="public-gallery-search"
      :placeholder="$t('apps.uploadsFileManager.searchPlaceholder')"
      @update:value="onKeywordInput"
    />
    <div v-if="images.length === 0 && !loading" class="public-empty">
      {{ keyword || category ? $t('apps.uploadsFileManager.noMatch') : $t('apps.publicGallery.empty') }}
    </div>
    <NImageGroup v-else>
      <NGrid class="mt-2" cols="2 300:2 600:4 900:6" :x-gap="5" :y-gap="5">
        <NGridItem v-for="item in images" :key="item.id">
          <div class="public-card" :data-filename="item.fileName">
            <NCard size="small" :bordered="true">
              <template #cover>
                <div class="public-preview">
                  <NImage style="object-fit: contain;height: 100%;" :src="item.src" />
                </div>
              </template>
              <template #footer>
                <span class="text-xs">
                  <NEllipsis>{{ item.fileName }}</NEllipsis>
                </span>
                <div class="public-actions">
                  <NButtonGroup>
                    <NButton v-if="selectable" size="tiny" type="primary" data-testid="public-gallery-pick" @click="useImage(item.src)">
                      {{ $t('apps.publicGallery.pick') }}
                    </NButton>
                    <NButton size="tiny" tertiary :title="$t('apps.uploadsFileManager.copyLink')" @click="copyImageUrl(item.src)">
                      <template #icon>
                        <SvgIcon icon="ion-copy" />
                      </template>
                    </NButton>
                    <NButton v-if="!selectable" size="tiny" tertiary :title="$t('apps.uploadsFileManager.setWallpaper')" @click="setWallpaper(item.src)">
                      <template #icon>
                        <SvgIcon icon="lucide:wallpaper" />
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
    <div v-if="total > 12" class="public-pages">
      <NPagination :page="page" :page-count="pageCount" @update:page="changePage" />
    </div>
  </div>
</template>

<style scoped>
.public-filters,
.public-pages,
.public-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

.public-filter {
  border: 1px solid rgba(148, 163, 184, 0.8);
  background: transparent;
  border-radius: 999px;
  padding: 4px 10px;
  cursor: pointer;
}

.public-filter.active {
  background: #0f172a;
  color: #f8fafc;
  border-color: #e2e8f0;
}

.public-empty {
  margin-top: 12px;
}

.public-preview {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 80px;
  background: rgba(148, 163, 184, 0.16);
}
</style>
