<template>
  <template v-if="isReader">
    <router-view />
    <GlobalOverlayHost v-if="isLoggedIn" />
  </template>

  <template v-else-if="isLoggedIn">
    <AppLayout>
      <router-view />
    </AppLayout>
    <GlobalOverlayHost />
  </template>

  <router-view v-else />
</template>

<script setup>
import { computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from './layouts/AppLayout.vue'
import GlobalOverlayHost from './components/GlobalOverlayHost.vue'
import { useUserStore } from './stores/user'
import { useReaderStore } from './stores/reader'
import { useSync } from './composables/useSync'

const route = useRoute()
const userStore = useUserStore()
const readerStore = useReaderStore()
const { connect, disconnect } = useSync()

const isLoggedIn = computed(() => !!userStore.token)
const isReader = computed(() => route.name === 'reader')

onMounted(() => {
  readerStore.normalizeSettings()
  if (userStore.token && !userStore.profile) {
    userStore.loadMe().catch(() => {})
  }
  if (userStore.token) {
    connect()
    readerStore.loadReaderSettings().catch(() => {})
  }
})

watch(isLoggedIn, (loggedIn) => {
  if (loggedIn) {
    connect()
    readerStore.loadReaderSettings().catch(() => {})
  } else {
    disconnect()
  }
})
</script>
