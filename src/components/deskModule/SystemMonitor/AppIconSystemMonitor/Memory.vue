<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import GenericProgress from '../components/GenericProgress/index.vue'
import { correctionNumberByCardStyle } from './common'
import { getMemonyState } from '@/api/system/systemMonitor'
import type { PanelPanelConfigStyleEnum } from '@/enums'
import { bytesToSize } from '@/utils/cmn'
import { createPoller } from '@/utils/monitor/pollLifecycle'

interface Prop {
  cardTypeStyle: PanelPanelConfigStyleEnum
  refreshInterval: number
  textColor: string
  progressColor: string
  progressRailColor: string
}

const props = defineProps<Prop>()
const memoryState = ref<SystemMonitor.MemoryInfo | null>(null)
const poller = createPoller({
  intervalMs: props.refreshInterval,
  run: async (isCurrent) => {
    const { data, code } = await getMemonyState<SystemMonitor.MemoryInfo>()
    if (!isCurrent() || code !== 0)
      return
    memoryState.value = data
  },
})

function formatMemorySize(v: number): string {
  return bytesToSize(v)
}

onMounted(() => {
  poller.start()
})

onUnmounted(() => {
  poller.stop()
})
</script>

<template>
  <GenericProgress
    :progress-color="progressColor"
    :progress-rail-color="progressRailColor"
    :progress-height="5"
    :percentage="correctionNumberByCardStyle(memoryState?.usedPercent || 0, cardTypeStyle)"
    :card-type-style="cardTypeStyle"
    :info-card-right-text="`${formatMemorySize(memoryState?.used || 0)}/${formatMemorySize((memoryState?.total || 0) - (memoryState?.used || 0) || 0)}`"
    info-card-left-text="RAM"
    :text-color="textColor"
  />
</template>
