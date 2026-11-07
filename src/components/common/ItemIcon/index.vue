<script setup lang="ts">
import { Icon, loadIcon } from '@iconify/vue'
import { NAvatar } from 'naive-ui'
import { computed, ref, watch, withDefaults } from 'vue'
import { t } from '@/locales'
import { fallbackLabel, isRemoteIconType, reduceRemoteIconPhase, rejectAfter, type RemoteIconPhase } from '@/utils/icon/remoteIcon'

interface Prop {
  itemIcon?: Panel.ItemIcon | null
  size?: number // 默认70
  forceBackground?: string // 强制背景色
  fallbackText?: string
}

const props = withDefaults(defineProps<Prop>(), { size: 70 })
const defaultBackground = '#2a2a2a6b'
const phase = ref<RemoteIconPhase>('loading')
let requestSerial = 0
const defaultStyle = computed(() => ({
  width: `${props.size}px`,
  height: `${props.size}px`,
}))
const iconExt = computed(() => {
  return props.itemIcon?.src?.split('.').pop()
})
const backgroundColor = computed(() => (props.forceBackground ?? props.itemIcon?.backgroundColor) || defaultBackground)
const letter = computed(() => fallbackLabel(props.itemIcon?.itemType, props.itemIcon?.text, props.fallbackText))
const remoteKey = computed(() => {
  if (!isRemoteIconType(props.itemIcon?.itemType))
    return ''
  if (props.itemIcon?.itemType === 2)
    return `image:${props.itemIcon.src || ''}`
  return `icon:${props.itemIcon?.text || ''}`
})
const showImage = computed(() => props.itemIcon?.itemType === 2 && phase.value !== 'fallback' && !!props.itemIcon.src)
const showOnlineIcon = computed(() => props.itemIcon?.itemType === 3 && phase.value === 'ready' && !!props.itemIcon.text)

watch(remoteKey, (key) => {
  const serial = ++requestSerial
  phase.value = 'loading'
  if (!key || props.itemIcon?.itemType !== 3)
    return
  const name = props.itemIcon.text?.trim()
  if (!name) {
    phase.value = reduceRemoteIconPhase(phase.value, 'error')
    return
  }
  void loadOnlineIcon(name, serial)
}, { immediate: true })

watch(() => props.itemIcon?.src, (src) => {
  if (props.itemIcon?.itemType === 2 && !src)
    phase.value = reduceRemoteIconPhase('loading', 'error')
}, { immediate: true })

async function loadOnlineIcon(name: string, serial: number) {
  try {
    await Promise.race([
      loadIcon(name),
      rejectAfter(8000),
    ])
    if (serial !== requestSerial)
      return
    phase.value = reduceRemoteIconPhase(phase.value, 'success')
  }
  catch {
    if (serial !== requestSerial)
      return
    phase.value = reduceRemoteIconPhase(phase.value, 'error')
  }
}

function onImgLoad() {
  phase.value = reduceRemoteIconPhase(phase.value, 'success')
}

function onImgError() {
  phase.value = reduceRemoteIconPhase(phase.value, 'error')
}
</script>

<template>
  <div class="item-icon" :style="defaultStyle">
    <slot>
      <template v-if="itemIcon">
        <template v-if="itemIcon.itemType === 1">
          <NAvatar :size="props.size" :style="{ backgroundColor }">
            {{ itemIcon.text }}
          </NAvatar>
        </template>

        <template v-else-if="showImage">
          <div :style="{ backgroundColor, ...defaultStyle }" class="flex justify-center items-center overflow-hidden">
            <img
              :src="itemIcon.src"
              :alt="fallbackText || letter"
              :class="iconExt === 'svg' ? 'w-[35px] h-[35px]' : 'w-full h-full object-cover'"
              @load="onImgLoad"
              @error="onImgError"
            >
          </div>
        </template>

        <template v-else-if="showOnlineIcon">
          <NAvatar :size="props.size" :style="{ backgroundColor }">
            <Icon style="font-size: 35px;" :icon="itemIcon.text || ''" />
          </NAvatar>
        </template>

        <template v-else>
          <NAvatar :size="props.size" :title="t('iconItem.iconLoadFailed')" :style="{ backgroundColor }">
            {{ letter }}
          </NAvatar>
        </template>
      </template>

      <template v-else>
        <NAvatar :size="props.size" />
      </template>
    </slot>
  </div>
</template>
