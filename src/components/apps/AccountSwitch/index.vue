<script setup lang="ts">
import { NAlert, NButton, NInput, useMessage } from 'naive-ui'
import { onMounted, ref } from 'vue'
import { login, logoutToken } from '@/api'
import { useAuthStore, usePanelState } from '@/store'
import { t } from '@/locales'
import { forgetAccount, rememberAccount, type AccountSession } from '@/utils/account/sessions'
import { loadAccountSessions, saveAccountSessions } from '@/utils/account/storage'

const authStore = useAuthStore()
const panelState = usePanelState()
const ms = useMessage()
const sessions = ref<AccountSession[]>([])
const username = ref('')
const password = ref('')
const loading = ref(false)

function reloadSessions() {
  sessions.value = loadAccountSessions()
}

function rememberCurrent() {
  if (!authStore.token || !authStore.userInfo?.id || !authStore.userInfo.username)
    return
  const next = rememberAccount(sessions.value, {
    id: authStore.userInfo.id,
    username: authStore.userInfo.username,
    name: authStore.userInfo.name,
    role: authStore.userInfo.role,
    token: authStore.token,
  })
  saveAccountSessions(next)
  sessions.value = next
}

function activate(session: AccountSession) {
  if (session.token === authStore.token)
    return
  panelState.removeState()
  authStore.setToken(session.token)
  authStore.setUserInfo({
    id: session.id,
    username: session.username,
    name: session.name,
    role: session.role,
  })
  location.reload()
}

async function addAccount() {
  const secret = password.value
  password.value = ''
  const account = username.value.trim()
  if (!account || !secret) {
    ms.error(t('apps.accountSwitch.loginFailed'))
    return
  }
  loading.value = true
  try {
    const res = await login<User.Info>({ username: account, password: secret })
    if (res.code !== 0 || !res.data?.token || !res.data.id) {
      ms.error(t('apps.accountSwitch.loginFailed'))
      return
    }
    const next = rememberAccount(sessions.value, res.data)
    saveAccountSessions(next)
    username.value = ''
    const saved = next.find(item => item.id === res.data.id)
    if (saved)
      activate(saved)
  }
  catch {
    ms.error(t('apps.accountSwitch.loginFailed'))
  }
  finally {
    loading.value = false
  }
}

async function forget(session: AccountSession) {
  await logoutToken(session.token)
  const next = forgetAccount(sessions.value, session.id)
  saveAccountSessions(next)
  sessions.value = next
  if (session.token !== authStore.token)
    return
  const fallback = next[0]
  if (fallback) {
    activate(fallback)
    return
  }
  panelState.removeState()
  authStore.removeToken()
  location.reload()
}

onMounted(() => {
  reloadSessions()
  rememberCurrent()
})
</script>

<template>
  <div class="p-3" data-testid="account-switch">
    <NAlert type="warning" :bordered="false">
      {{ $t('apps.accountSwitch.warning') }}
    </NAlert>
    <div class="account-list">
      <button
        v-for="item in sessions"
        :key="item.id"
        type="button"
        class="account-session"
        :class="{ active: item.token === authStore.token }"
        :data-username="item.username"
        @click="activate(item)"
      >
        <span>{{ item.name }}</span>
        <span class="account-meta">{{ item.username }}</span>
        <span v-if="item.token === authStore.token" class="account-meta">{{ $t('apps.accountSwitch.current') }}</span>
      </button>
    </div>
    <div class="account-add">
      <NInput v-model:value="username" data-testid="account-switch-username" :placeholder="$t('login.usernamePlaceholder')" />
      <NInput v-model:value="password" data-testid="account-switch-password" type="password" show-password-on="click" :placeholder="$t('login.passwordPlaceholder')" @keydown.enter="addAccount" />
      <NButton type="primary" data-testid="account-switch-add" :loading="loading" @click="addAccount">
        {{ $t('apps.accountSwitch.add') }}
      </NButton>
    </div>
    <div class="account-forget">
      <NButton
        v-for="item in sessions"
        :key="`forget-${item.id}`"
        quaternary
        size="small"
        @click="forget(item)"
      >
        {{ $t('apps.accountSwitch.forget', { name: item.name }) }}
      </NButton>
    </div>
  </div>
</template>

<style scoped>
.account-list,
.account-add,
.account-forget {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
}

.account-session {
  text-align: left;
  border: 1px solid rgba(148, 163, 184, 0.8);
  background: transparent;
  border-radius: 12px;
  padding: 8px 10px;
  cursor: pointer;
}

.account-session.active {
  background: #0f172a;
  color: #f8fafc;
  border-color: #e2e8f0;
}

.account-meta {
  display: block;
  font-size: 12px;
  opacity: 0.8;
}
</style>
