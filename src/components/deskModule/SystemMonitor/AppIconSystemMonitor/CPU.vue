<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import GenericProgress from '../components/GenericProgress/index.vue'
import { correctionNumber, correctionNumberByCardStyle } from './common'
import { getCpuState } from '@/api/system/systemMonitor'
import type { PanelPanelConfigStyleEnum } from '@/enums'
import { attachMonitorPoll, firstNumber } from '@/utils/monitor/pollLifecycle'

interface Prop {
  cardTypeStyle: PanelPanelConfigStyleEnum
  refreshInterval: number
  textColor: string
  progressColor: string
  progressRailColor: string
}

const props = defineProps<Prop>()
const cpuState = ref<SystemMonitor.CPUInfo | null>(null)
const usage = computed(() => firstNumber(cpuState.value?.usages))
attachMonitorPoll({
  intervalMs: props.refreshInterval,
  run: async (isCurrent) => {
    const { data, code } = await getCpuState<SystemMonitor.CPUInfo>()
    if (!isCurrent() || code !== 0)
      return
    cpuState.value = data
  },
  onMounted,
  onUnmounted,
})
</script>

<template>
  <GenericProgress
    :progress-color="progressColor"
    :progress-rail-color="progressRailColor"
    :progress-height="5"
    :percentage="correctionNumberByCardStyle(usage, cardTypeStyle)"
    :card-type-style="cardTypeStyle"
    :info-card-right-text="`${correctionNumber(usage)}%`"
    info-card-left-text="CPU"
    :text-color="textColor"
    style="width: 100%;"
  />
</template>
