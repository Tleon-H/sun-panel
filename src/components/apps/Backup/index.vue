<script setup lang="ts">
import { NAlert, NButton, useMessage } from 'naive-ui'
import { computed, ref } from 'vue'
import { downloadBackup, restoreBackup } from '@/api/system/backup'
import { t } from '@/locales'

const downloading = ref(false)
const restoring = ref(false)
const showConfirm = ref(false)
const acknowledged = ref(false)
const file = ref<File | null>(null)
const ms = useMessage()
const busy = computed(() => downloading.value || restoring.value)

async function onDownload() {
  if (busy.value)
    return
  downloading.value = true
  try {
    await downloadBackup()
    ms.success(t('apps.backup.started'))
  }
  catch (error) {
    const message = error instanceof Error ? error.message : ''
    ms.error(message || t('common.failed'))
  }
  finally {
    downloading.value = false
  }
}

function onFile(event: Event) {
  const input = event.target as HTMLInputElement
  file.value = input.files && input.files.length > 0 ? input.files[0] : null
  acknowledged.value = false
  showConfirm.value = false
}

function openConfirm() {
  if (!file.value || busy.value)
    return
  acknowledged.value = false
  showConfirm.value = true
}

function closeConfirm() {
  if (restoring.value)
    return
  showConfirm.value = false
  acknowledged.value = false
}

async function onRestore() {
  if (!file.value || !acknowledged.value || busy.value)
    return
  restoring.value = true
  try {
    await restoreBackup(file.value)
    showConfirm.value = false
    ms.success(t('apps.backup.restored'))
    window.location.reload()
  }
  catch (error) {
    const message = error instanceof Error ? error.message : ''
    ms.error(message || t('common.failed'))
  }
  finally {
    restoring.value = false
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
        :loading="downloading"
        :disabled="busy"
        @click="onDownload"
      >
        {{ downloading ? $t('apps.backup.downloading') : $t('apps.backup.download') }}
      </NButton>
    </div>

    <NAlert class="mt-4" type="error" :bordered="false" :title="$t('apps.backup.restoreTitle')" data-testid="backup-restore-risk">
      {{ $t('apps.backup.restoreRisk') }}
    </NAlert>
    <div class="mt-3">
      <label class="block text-sm" for="backup-restore-file">{{ $t('apps.backup.choose') }}</label>
      <input
        id="backup-restore-file"
        class="mt-2 block w-full text-sm"
        data-testid="backup-restore-file"
        type="file"
        accept=".zip,application/zip"
        :disabled="busy"
        @change="onFile"
      >
      <p v-if="file" class="mt-2 text-sm break-all" data-testid="backup-restore-filename">
        {{ file.name }}
      </p>
      <NButton
        class="mt-3"
        type="error"
        data-testid="backup-restore-open"
        :disabled="!file || busy"
        @click="openConfirm"
      >
        {{ $t('apps.backup.restore') }}
      </NButton>
    </div>

    <div v-if="showConfirm" class="mt-3 rounded-lg border border-red-300 p-3" data-testid="backup-restore-confirm-panel">
      <p class="font-bold" data-testid="backup-restore-confirm-title">
        {{ $t('apps.backup.restoreConfirmTitle') }}
      </p>
      <p class="mt-2" data-testid="backup-restore-confirm-body">
        {{ $t('apps.backup.restoreConfirmBody') }}
      </p>
      <p v-if="file" class="mt-2 text-sm break-all">
        {{ file.name }}
      </p>
      <label class="mt-3 flex items-center gap-2" data-testid="backup-restore-ack">
        <input v-model="acknowledged" type="checkbox">
        <span>{{ $t('apps.backup.acknowledge') }}</span>
      </label>
      <div class="mt-3 flex gap-2">
        <NButton data-testid="backup-restore-cancel" :disabled="restoring" @click="closeConfirm">
          {{ $t('apps.backup.cancel') }}
        </NButton>
        <NButton
          type="error"
          data-testid="backup-restore-confirm"
          :disabled="!acknowledged || restoring"
          :loading="restoring"
          @click="onRestore"
        >
          {{ restoring ? $t('apps.backup.restoring') : $t('apps.backup.confirmRestore') }}
        </NButton>
      </div>
    </div>
  </div>
</template>
