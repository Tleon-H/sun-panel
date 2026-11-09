<script setup lang="ts">
import { NAlert, NButton, useMessage } from 'naive-ui'
import { ref } from 'vue'
import { downloadBackup } from '@/api/system/backup'
import { t } from '@/locales'

const loading = ref(false)
const ms = useMessage()

async function onDownload() {
  if (loading.value)
    return
  loading.value = true
  try {
    await downloadBackup()
    ms.success(t('apps.backup.started'))
  }
  catch (error) {
    const message = error instanceof Error ? error.message : ''
    ms.error(message || t('common.failed'))
  }
  finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="p-3">
    <NAlert type="warning" :bordered="false" :title="$t('apps.backup.appName')">
      {{ $t('apps.backup.warning') }}
    </NAlert>
    <div class="mt-3">
      <NButton
        type="primary"
        data-testid="backup-download"
        :loading="loading"
        @click="onDownload"
      >
        {{ loading ? $t('apps.backup.downloading') : $t('apps.backup.download') }}
      </NButton>
    </div>
  </div>
</template>
