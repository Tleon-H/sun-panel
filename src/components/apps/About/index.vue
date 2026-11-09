<script setup lang="ts">
import { NTag } from 'naive-ui'
import { onMounted, ref } from 'vue'
import { get } from '@/api/system/about'
import srcSvglogo from '@/assets/logo.svg'

interface Version {
  versionName: string
  versionCode: number
}

const versionName = ref('')
const frontVersion = import.meta.env.VITE_APP_VERSION || 'unknown'

onMounted(() => {
  get<Version>().then((res) => {
    if (res.code === 0)
      versionName.value = res.data.versionName
  })
})
</script>

<template>
  <div class="pt-5">
    <div class="flex flex-col items-center justify-center">
      <img :src="srcSvglogo" width="100" height="100" alt="Leon Panel">
      <div class="text-3xl font-semibold">
        {{ $t('common.appName') }}
      </div>
      <div class="text-xl">
        <span class="font-semibold">v{{ versionName }}</span>
      </div>
      <p class="mt-2 max-w-[520px] px-4 text-center text-sm leading-6">
        {{ $t('apps.about.independentNote') }}
      </p>
      <div class="mt-2">
        {{ $t('apps.about.upstreamProject') }}
        <a href="https://github.com/hslr-s/sun-panel" target="_blank" class="link">Sun-Panel</a>
      </div>
      <div class="mt-1">
        {{ $t('apps.about.author') }}<a href="https://github.com/hslr-s" target="_blank" class="link">红烧猎人</a>
      </div>
      <div class="mt-1">
        {{ $t('apps.about.repository') }}
        <a href="https://github.com/Tleon-H/sun-panel" target="_blank" class="link">Tleon-H/sun-panel</a>
      </div>
      <div class="mt-1">
        {{ $t('apps.about.license') }}
        <a href="https://github.com/hslr-s/sun-panel/blob/master/LICENSE" target="_blank" class="link">MIT</a>
      </div>
    </div>

    <div class="mt-5 flex justify-center">
      <NTag :bordered="false" size="small">
        {{ $t("apps.about.frontVersionText") }}: FV-{{ frontVersion }}
      </NTag>
    </div>
  </div>
</template>

<style>
.link{
    color:rgb(0, 89, 255)
}
</style>
