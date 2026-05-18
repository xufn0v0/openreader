<template>
  <div class="app-shell">
    <div v-if="offline" class="app-offline">网络已断开，部分同步能力会在恢复连接后继续。</div>

    <aside class="app-sidebar">
      <div class="app-brand" @click="goHome">
        <div class="app-brand-mark">阅</div>
        <div>
          <div class="app-brand-title">OpenReader</div>
          <div class="app-brand-subtitle">自部署阅读空间</div>
        </div>
      </div>

      <div class="app-shell-search">
        <el-input
          v-model="quickSearch"
          placeholder="搜索远程书籍"
          clearable
          @keyup.enter="goSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
      </div>

      <nav class="app-nav">
        <button
          v-for="item in navItems"
          :key="item.name"
          class="app-nav-item"
          :class="{ active: route.name === item.name }"
          type="button"
          @click="goRoute(item.name)"
        >
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ item.label }}</span>
        </button>
      </nav>

      <div class="app-sidebar-footer">
        <div class="sync-pill" :class="{ connected: syncConnected }">
          <span class="sync-dot" />
          {{ syncConnected ? '实时同步在线' : '同步未连接' }}
        </div>

        <el-dropdown trigger="click" placement="top-start">
          <button class="user-card" type="button">
            <el-avatar :size="34">{{ userInitial }}</el-avatar>
            <span class="user-card-main">
              <strong>{{ userStore.profile?.username || '用户' }}</strong>
              <small>账户与设置</small>
            </span>
            <el-icon><ArrowUp /></el-icon>
          </button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item @click="goRoute('settings')">
                <el-icon><Setting /></el-icon>
                设置
              </el-dropdown-item>
              <el-dropdown-item divided @click="handleLogout">
                <el-icon><SwitchButton /></el-icon>
                退出登录
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </aside>

    <div class="app-workspace">
      <header class="mobile-topbar">
        <button class="mobile-brand" type="button" @click="goHome">
          <span class="app-brand-mark">阅</span>
          <span>OpenReader</span>
        </button>
        <div class="mobile-actions">
          <button class="mobile-icon-btn" type="button" @click="goRoute('search')" title="搜索">
            <el-icon><Search /></el-icon>
          </button>
          <button class="mobile-icon-btn" type="button" @click="goRoute('settings')" title="设置">
            <el-icon><Setting /></el-icon>
          </button>
        </div>
      </header>

      <main class="app-content">
        <slot />
      </main>
    </div>

    <nav class="mobile-nav">
      <button
        v-for="item in mobilePrimaryNavItems"
        :key="item.name"
        class="mobile-nav-item"
        :class="{ active: route.name === item.name }"
        type="button"
        @click="goRoute(item.name)"
      >
        <el-icon><component :is="item.icon" /></el-icon>
        <span>{{ item.label }}</span>
      </button>
      <button class="mobile-nav-item" type="button" @click="mobileMenu = true">
        <el-icon><MoreFilled /></el-icon>
        <span>更多</span>
      </button>
    </nav>

    <el-drawer v-model="mobileMenu" title="全部功能" direction="btt" size="72%" class="mobile-menu-drawer">
      <div class="mobile-menu-grid">
        <button
          v-for="item in navItems"
          :key="item.name"
          class="mobile-menu-item"
          :class="{ active: route.name === item.name }"
          type="button"
          @click="goRoute(item.name)"
        >
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ item.label }}</span>
        </button>
      </div>
      <div class="mobile-menu-status">
        <span class="sync-pill" :class="{ connected: syncConnected }">
          <span class="sync-dot" />
          {{ syncConnected ? '实时同步在线' : '同步未连接' }}
        </span>
        <button class="mobile-logout" type="button" @click="handleLogout">退出登录</button>
      </div>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ArrowUp,
  Compass,
  Connection,
  FolderOpened,
  MoreFilled,
  Notebook,
  Search,
  Setting,
  SwitchButton,
} from '@element-plus/icons-vue'
import { useUserStore } from '../stores/user'
import { useSync } from '../composables/useSync'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const quickSearch = ref('')
const offline = ref(false)
const mobileMenu = ref(false)

const navItems = [
  { name: 'home', label: '书架', icon: Notebook },
  { name: 'search', label: '搜索', icon: Search },
  { name: 'discover', label: '书海', icon: Compass },
  { name: 'sources', label: '书源', icon: Connection },
  { name: 'local-store', label: '书仓', icon: FolderOpened },
  { name: 'settings', label: '设置', icon: Setting },
]

const mobilePrimaryNavItems = navItems.filter(item => ['home', 'search', 'discover'].includes(item.name))
const userInitial = computed(() => (userStore.profile?.username || '?').slice(0, 1).toUpperCase())

const { connected: syncConnected, connect, disconnect } = useSync()

function goHome() {
  router.push({ name: 'home' })
}

function goRoute(name) {
  mobileMenu.value = false
  router.push({ name })
}

function goSearch() {
  const keyword = quickSearch.value.trim()
  if (!keyword) return
  router.push({ name: 'search', query: { q: keyword } })
  quickSearch.value = ''
}

function handleLogout() {
  mobileMenu.value = false
  userStore.logout()
  router.push({ name: 'login' })
}

function setOffline() {
  offline.value = true
}

function setOnline() {
  offline.value = false
}

watch(
  () => userStore.token,
  (token) => {
    if (token) {
      connect()
    } else {
      disconnect()
    }
  },
  { immediate: true },
)

onMounted(() => {
  window.addEventListener('offline', setOffline)
  window.addEventListener('online', setOnline)
  offline.value = !navigator.onLine
  if (userStore.token && !userStore.profile) {
    userStore.loadMe().catch(() => {})
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('offline', setOffline)
  window.removeEventListener('online', setOnline)
})
</script>

<style scoped>
.app-shell {
  min-height: 100vh;
  background:
    linear-gradient(180deg, rgba(255, 253, 248, 0.86), rgba(245, 241, 232, 0.92)),
    var(--app-bg);
}

.app-offline {
  position: fixed;
  z-index: 50;
  top: 12px;
  left: 50%;
  transform: translateX(-50%);
  padding: 8px 14px;
  color: #fff9ed;
  background: var(--app-warning);
  border-radius: 999px;
  box-shadow: var(--app-shadow-md);
  font-size: 13px;
}

.app-sidebar {
  position: fixed;
  inset: 0 auto 0 0;
  z-index: 30;
  display: flex;
  width: var(--app-sidebar-width);
  flex-direction: column;
  padding: 18px 14px;
  color: #fff9ed;
  background: var(--app-nav-bg);
  border-right: 1px solid rgba(255, 249, 237, 0.08);
}

.app-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 8px 18px;
  cursor: pointer;
}

.app-brand-mark {
  display: inline-grid;
  width: 38px;
  height: 38px;
  place-items: center;
  flex: 0 0 38px;
  color: #2b2118;
  background: var(--app-nav-active);
  border-radius: var(--app-radius-md);
  font-weight: 800;
}

.app-brand-title {
  font-size: 17px;
  font-weight: 800;
  line-height: 1.2;
}

.app-brand-subtitle {
  margin-top: 3px;
  color: var(--app-nav-muted);
  font-size: 12px;
}

.app-shell-search {
  margin: 0 4px 18px;
}

.app-nav {
  display: grid;
  gap: 4px;
}

.app-nav-item {
  display: flex;
  width: 100%;
  height: 42px;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  color: var(--app-nav-muted);
  background: transparent;
  border: 0;
  border-radius: var(--app-radius-sm);
  cursor: pointer;
  text-align: left;
}

.app-nav-item:hover {
  color: #fff9ed;
  background: rgba(255, 249, 237, 0.07);
}

.app-nav-item.active {
  color: var(--app-nav-active);
  background: rgba(244, 228, 197, 0.12);
}

.app-sidebar-footer {
  display: grid;
  gap: 10px;
  margin-top: auto;
}

.sync-pill {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 7px 9px;
  color: var(--app-nav-muted);
  background: rgba(255, 249, 237, 0.06);
  border-radius: 999px;
  font-size: 12px;
}

.sync-pill.connected {
  color: #bfe6c9;
}

.sync-dot {
  width: 7px;
  height: 7px;
  background: #8d8174;
  border-radius: 50%;
}

.sync-pill.connected .sync-dot {
  background: #77c78b;
}

.user-card {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  padding: 9px;
  color: #fff9ed;
  background: rgba(255, 249, 237, 0.07);
  border: 1px solid rgba(255, 249, 237, 0.08);
  border-radius: var(--app-radius-md);
  cursor: pointer;
}

.user-card-main {
  display: grid;
  min-width: 0;
  flex: 1;
  text-align: left;
}

.user-card-main strong,
.user-card-main small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-card-main small {
  margin-top: 2px;
  color: var(--app-nav-muted);
  font-size: 12px;
}

.app-workspace {
  min-height: 100vh;
  padding-left: var(--app-sidebar-width);
}

.app-content {
  min-height: 100vh;
}

.mobile-topbar,
.mobile-nav {
  display: none;
}

@media (max-width: 860px) {
  .app-sidebar {
    display: none;
  }

  .app-workspace {
    padding-left: 0;
  }

  .mobile-topbar {
    position: sticky;
    top: 0;
    z-index: 25;
    display: flex;
    height: 58px;
    align-items: center;
    justify-content: space-between;
    padding: 0 14px;
    background: rgba(255, 253, 248, 0.92);
    border-bottom: 1px solid var(--app-border);
    backdrop-filter: blur(14px);
  }

  .mobile-brand {
    display: flex;
    align-items: center;
    gap: 9px;
    color: var(--app-text);
    background: transparent;
    border: 0;
    font-weight: 800;
  }

  .mobile-brand .app-brand-mark {
    width: 32px;
    height: 32px;
    flex-basis: 32px;
    background: var(--app-primary);
    color: #fff;
  }

  .mobile-actions {
    display: flex;
    gap: 6px;
  }

  .mobile-icon-btn {
    display: grid;
    width: 36px;
    height: 36px;
    place-items: center;
    color: var(--app-text);
    background: var(--app-surface);
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius-sm);
  }

  .mobile-nav {
    position: fixed;
    right: 10px;
    bottom: 10px;
    left: 10px;
    z-index: 40;
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    padding: 6px;
    background: rgba(37, 34, 31, 0.94);
    border: 1px solid rgba(255, 249, 237, 0.1);
    border-radius: 16px;
    box-shadow: var(--app-shadow-md);
    backdrop-filter: blur(14px);
  }

  .mobile-nav-item {
    display: grid;
    min-width: 0;
    gap: 3px;
    place-items: center;
    padding: 7px 2px 6px;
    color: var(--app-nav-muted);
    background: transparent;
    border: 0;
    border-radius: 12px;
    font-size: 11px;
  }

  .mobile-nav-item.active {
    color: var(--app-nav-active);
    background: rgba(244, 228, 197, 0.12);
  }

  .mobile-menu-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 10px;
  }

  .mobile-menu-item {
    display: grid;
    min-height: 74px;
    place-items: center;
    gap: 6px;
    color: var(--app-text);
    background: var(--app-bg-soft);
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius-sm);
  }

  .mobile-menu-item.active {
    color: var(--app-primary-strong);
    background: var(--app-primary-soft);
    border-color: var(--app-primary);
  }

  .mobile-menu-status {
    display: grid;
    gap: 10px;
    margin-top: 18px;
  }

  .mobile-menu-status .sync-pill {
    justify-content: center;
    color: var(--app-text-muted);
    background: var(--app-bg-soft);
  }

  .mobile-logout {
    height: 42px;
    color: var(--app-danger);
    background: var(--app-surface);
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius-sm);
  }
}
</style>
