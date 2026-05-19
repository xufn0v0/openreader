<template>
  <section class="app-page shelf-page">
    <header class="shelf-head">
      <div>
        <h1 class="app-page-title">书架 ({{ displayedBooks.length }})</h1>
      </div>
    </header>

    <section v-if="recentBook" class="recent-panel app-panel">
      <div class="recent-cover" :style="coverStyle(recentBook)">{{ coverInitial(recentBook) }}</div>
      <div class="recent-main">
        <span>最近阅读</span>
        <h2>{{ recentBook.title }}</h2>
        <p>{{ recentBook.author || '未知作者' }} · {{ recentBook.lastChapter || '暂无最新章节' }}</p>
      </div>
      <el-button type="primary" @click="continueRead(recentBook)">继续阅读</el-button>
    </section>

    <div class="shelf-layout">
      <aside class="group-panel app-panel">
        <div class="group-title">
          <strong>书架分组</strong>
          <el-button text :icon="Plus" @click="overlay.openBookGroup('manage')">管理</el-button>
        </div>
        <div v-if="showGroupInput" class="group-create">
          <el-input v-model="newGroupName" placeholder="分组名称" size="small" @keyup.enter="createCategory" />
          <el-button size="small" type="primary" :disabled="!newGroupName.trim()" @click="createCategory">保存</el-button>
        </div>
        <div
          v-for="item in groupItems"
          :key="item.id"
          class="group-row"
          :class="{ active: selectedGroup === item.id }"
        >
          <button type="button" class="group-item" @click="selectGroup(item.id)">
            <span>{{ item.name }}</span>
            <small>{{ item.count }}</small>
          </button>
          <span v-if="!item.builtin" class="group-actions">
            <button type="button" title="上移" @click="moveGroup(item, -1)">
              <el-icon><ArrowUp /></el-icon>
            </button>
            <button type="button" title="下移" @click="moveGroup(item, 1)">
              <el-icon><ArrowDown /></el-icon>
            </button>
            <button type="button" title="重命名" @click="renameGroup(item)">
              <el-icon><Edit /></el-icon>
            </button>
            <button type="button" title="删除" @click="deleteGroup(item)">
              <el-icon><Delete /></el-icon>
            </button>
          </span>
        </div>
        <el-alert
          class="group-note"
          type="info"
          :closable="false"
          show-icon
          title="分组已支持新增、重命名、排序和删除；批量分组在书籍管理中处理。"
        />
      </aside>

      <main class="shelf-main">
        <div class="shelf-toolbar app-panel">
          <el-input v-model="keyword" placeholder="搜索书名或作者" clearable>
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-segmented v-model="viewMode" :options="viewOptions" />
          <el-button :icon="Search" @click="router.push({ name: 'search' })">远程搜书</el-button>
        </div>

        <div v-if="bookshelf.loading" class="book-grid">
          <article v-for="i in 8" :key="i" class="book-card app-panel skeleton-card">
            <el-skeleton :rows="4" animated />
          </article>
        </div>

        <template v-else-if="displayedBooks.length">
          <div v-if="viewMode === 'grid'" class="book-grid">
            <article v-for="book in displayedBooks" :key="book.id" class="book-card app-panel" @click="openDetail(book)">
              <div class="book-cover" :style="coverStyle(book)">{{ coverInitial(book) }}</div>
              <div class="book-body">
                <div class="book-title-row">
                  <h2>{{ book.title }}</h2>
                  <el-tag size="small" effect="plain" :type="book.sourceId ? 'success' : 'info'">
                    {{ book.sourceId ? '远程' : '本地' }}
                  </el-tag>
                </div>
                <p>{{ book.author || '未知作者' }}</p>
                <p class="book-latest">{{ book.lastChapter || '暂无最新章节' }}</p>
                <div class="book-meta">
                  <span>{{ book.chapterCount || 0 }} 章</span>
                  <span>{{ categoryName(book.categoryId) }}</span>
                </div>
                <el-progress
                  class="book-progress"
                  :percentage="bookProgressPercent(book)"
                  :stroke-width="5"
                  :show-text="false"
                />
              </div>
              <div class="book-actions" @click.stop>
                <el-button size="small" type="primary" plain @click="continueRead(book)">阅读</el-button>
                <el-button size="small" text @click="openDetail(book)">详情</el-button>
              </div>
            </article>
          </div>

          <div v-else class="book-list app-panel">
            <button v-for="book in displayedBooks" :key="book.id" class="book-row" type="button" @click="openDetail(book)">
              <span class="list-cover" :style="coverStyle(book)">{{ coverInitial(book) }}</span>
              <span class="list-main">
                <strong>{{ book.title }}</strong>
                <small>{{ book.author || '未知作者' }} · {{ categoryName(book.categoryId) }} · {{ bookProgressPercent(book) }}%</small>
              </span>
              <span class="list-latest">{{ book.lastChapter || '暂无最新章节' }}</span>
              <el-button size="small" type="primary" plain @click.stop="continueRead(book)">阅读</el-button>
            </button>
          </div>
        </template>

        <div v-else class="empty-panel app-panel">
          <el-empty :description="emptyText">
            <div class="empty-actions">
              <el-button type="primary" :icon="Upload" @click="importDialog = true">导入本地书</el-button>
              <el-button :icon="Search" @click="router.push({ name: 'search' })">搜索远程书</el-button>
            </div>
          </el-empty>
        </div>
      </main>
    </div>

    <el-dialog v-model="importDialog" title="导入本地书籍" width="520px">
      <div class="import-form">
        <el-upload drag :show-file-list="false" :auto-upload="false" accept=".txt,.text,.md,.epub,.pdf,.umd" @change="pickFile">
          <el-icon class="upload-icon"><UploadFilled /></el-icon>
          <div class="upload-text">{{ draft.file ? draft.file.name : '拖入或选择 TXT / EPUB / PDF / UMD 文件' }}</div>
        </el-upload>
        <el-input v-model="draft.title" placeholder="书名（可选，不填则使用文件名）" />
        <el-input v-model="draft.author" placeholder="作者（可选）" />
        <el-select v-model="draft.categoryId" placeholder="分组（可选）" clearable>
          <el-option label="未分组" value="" />
          <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
        </el-select>
      </div>
      <template #footer>
        <el-button @click="importDialog = false">取消</el-button>
        <el-button type="primary" :loading="importing" :disabled="!draft.file" @click="importBook">导入</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, ArrowUp, Delete, Edit, Plus, Search, Upload, UploadFilled } from '@element-plus/icons-vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'

const router = useRouter()
const route = useRoute()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()

const keyword = ref('')
const selectedGroup = ref('')
const viewMode = ref('grid')
const showGroupInput = ref(false)
const newGroupName = ref('')
const importDialog = ref(false)
const manageDrawer = ref(false)
const importing = ref(false)
const selectedBookIds = ref([])
const batchCategoryId = ref('')
const batchManaging = ref(false)
const draft = reactive({ title: '', author: '', categoryId: '', file: null })

const viewOptions = [
  { label: '网格', value: 'grid' },
  { label: '列表', value: 'list' },
]

const remoteCount = computed(() => bookshelf.books.filter(book => book.sourceId).length)
const localCount = computed(() => bookshelf.books.length - remoteCount.value)
const allManagedSelected = computed(() => bookshelf.books.length > 0 && selectedBookIds.value.length === bookshelf.books.length)
const someManagedSelected = computed(() => selectedBookIds.value.length > 0 && selectedBookIds.value.length < bookshelf.books.length)
const recentBook = computed(() => {
  const withProgress = bookshelf.books
    .filter(book => bookProgress(book))
    .sort((a, b) => new Date(bookProgress(b)?.updatedAt || 0) - new Date(bookProgress(a)?.updatedAt || 0))
  return withProgress[0] || bookshelf.books[0] || null
})

const groupItems = computed(() => {
  const countByCategory = new Map()
  for (const book of bookshelf.books) {
    const key = book.categoryId ? String(book.categoryId) : 'none'
    countByCategory.set(key, (countByCategory.get(key) || 0) + 1)
  }
  return [
    { id: '', name: '全部', count: bookshelf.books.length, builtin: true },
    { id: 'none', name: '未分组', count: countByCategory.get('none') || 0, builtin: true },
    ...bookshelf.categories.map(category => ({
      id: String(category.id),
      name: category.name,
      count: countByCategory.get(String(category.id)) || 0,
      sortOrder: category.sortOrder || 0,
      builtin: false,
    })),
  ]
})

const displayedBooks = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  return bookshelf.books
    .filter(book => {
      const matchesKeyword = !value || `${book.title || ''} ${book.author || ''}`.toLowerCase().includes(value)
      if (!matchesKeyword) return false
      if (!selectedGroup.value) return true
      if (selectedGroup.value === 'none') return !book.categoryId
      return String(book.categoryId) === selectedGroup.value
    })
    .sort(compareByReadingOrder)
})

const emptyText = computed(() => {
  if (keyword.value.trim()) return '没有匹配的书籍'
  if (selectedGroup.value) return '这个分组里还没有书'
  return '书架还是空的，导入一本书或搜索远程书源开始阅读'
})

onMounted(async () => {
  try {
    await Promise.all([bookshelf.loadCategories(), bookshelf.loadBooks()])
  } catch (err) {
    ElMessage.error(readError(err, '加载书架失败'))
  }
})

watch(
  () => route.query.import,
  (value) => {
    if (value === '1') importDialog.value = true
  },
  { immediate: true },
)

function selectGroup(groupId) {
  selectedGroup.value = groupId
}

async function createCategory() {
  const name = newGroupName.value.trim()
  if (!name) return
  try {
    await bookshelf.addCategory({ name })
    newGroupName.value = ''
    showGroupInput.value = false
    ElMessage.success('分组已创建')
  } catch (err) {
    ElMessage.error(readError(err, '创建分组失败'))
  }
}

async function renameGroup(item) {
  try {
    const { value } = await ElMessageBox.prompt('输入新的分组名称', '重命名分组', {
      inputValue: item.name,
      inputValidator: value => !!value?.trim() || '分组名称不能为空',
    })
    const name = value.trim()
    if (!name || name === item.name) return
    await bookshelf.renameCategory(Number(item.id), { name })
    ElMessage.success('分组已重命名')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '重命名失败'))
  }
}

async function deleteGroup(item) {
  try {
    await ElMessageBox.confirm(`确定删除分组“${item.name}”吗？组内书籍会移动到未分组。`, '删除分组', { type: 'warning' })
    await bookshelf.removeCategory(Number(item.id))
    if (selectedGroup.value === item.id) selectedGroup.value = ''
    ElMessage.success('分组已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除分组失败'))
  }
}

async function moveGroup(item, direction) {
  const categories = [...bookshelf.categories]
  const index = categories.findIndex(category => String(category.id) === String(item.id))
  const targetIndex = index + direction
  if (index < 0 || targetIndex < 0 || targetIndex >= categories.length) return
  const [moved] = categories.splice(index, 1)
  categories.splice(targetIndex, 0, moved)
  try {
    await bookshelf.reorderCategoryIds(categories.map(category => category.id))
    ElMessage.success('分组排序已更新')
  } catch (err) {
    ElMessage.error(readError(err, '分组排序失败'))
  }
}

function pickFile(data) {
  draft.file = data.raw || null
  if (draft.file && !draft.title) {
    draft.title = draft.file.name.replace(/\.[^.]+$/, '')
  }
}

async function importBook() {
  if (!draft.file) return
  importing.value = true
  try {
    const book = await bookshelf.importTXT({
      file: draft.file,
      title: draft.title,
      author: draft.author,
      categoryId: draft.categoryId,
    })
    ElMessage.success(`已导入《${book.title}》，共 ${book.chapterCount || 0} 章`)
    Object.assign(draft, { title: '', author: '', categoryId: '', file: null })
    importDialog.value = false
  } catch (err) {
    ElMessage.error(readError(err, '导入失败'))
  } finally {
    importing.value = false
  }
}

async function deleteManagedBook(book) {
  try {
    await ElMessageBox.confirm(`确定删除《${book.title}》吗？阅读进度和书签也会一并删除。`, '删除书籍', { type: 'warning' })
    await bookshelf.removeBook(book.id)
    ElMessage.success('书籍已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除失败'))
  }
}

function toggleManageAll(value) {
  selectedBookIds.value = value ? bookshelf.books.map(book => book.id) : []
}

async function batchSetCategory() {
  if (!selectedBookIds.value.length) return
  batchManaging.value = true
  try {
    const categoryId = batchCategoryId.value ? Number(batchCategoryId.value) : null
    await bookshelf.batchSetCategory([...selectedBookIds.value], categoryId)
    ElMessage.success(`已更新 ${selectedBookIds.value.length} 本书的分组`)
    selectedBookIds.value = []
  } catch (err) {
    ElMessage.error(readError(err, '批量分组失败'))
  } finally {
    batchManaging.value = false
  }
}

async function batchCacheBooks() {
  if (!selectedBookIds.value.length) return
  batchManaging.value = true
  try {
    const data = await bookshelf.batchCacheBooks([...selectedBookIds.value])
    ElMessage.success(`已处理 ${data.affected || 0} 本书，缓存 ${data.cached || 0}/${data.requested || 0} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '批量缓存失败'))
  } finally {
    batchManaging.value = false
  }
}

async function batchClearCache() {
  if (!selectedBookIds.value.length) return
  try {
    await ElMessageBox.confirm(`确定清理选中 ${selectedBookIds.value.length} 本书的章节缓存吗？`, '清理缓存', { type: 'warning' })
    batchManaging.value = true
    const data = await bookshelf.batchClearCache([...selectedBookIds.value])
    ElMessage.success(`已清理 ${data.cleared || 0} 个章节缓存`)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    batchManaging.value = false
  }
}

async function batchExportBooks() {
  if (!selectedBookIds.value.length) return
  batchManaging.value = true
  try {
    const blob = await bookshelf.exportSelectedBooks([...selectedBookIds.value])
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `openreader-books-${new Date().toISOString().slice(0, 10)}.json`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
    ElMessage.success(`已导出 ${selectedBookIds.value.length} 本书`)
  } catch (err) {
    ElMessage.error(readError(err, '批量导出失败'))
  } finally {
    batchManaging.value = false
  }
}

async function batchDeleteBooks() {
  if (!selectedBookIds.value.length) return
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${selectedBookIds.value.length} 本书吗？阅读进度和书签也会一并删除。`, '批量删除', { type: 'warning' })
    batchManaging.value = true
    await bookshelf.batchDeleteBooks([...selectedBookIds.value])
    ElMessage.success('已批量删除')
    selectedBookIds.value = []
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '批量删除失败'))
  } finally {
    batchManaging.value = false
  }
}

function openDetail(book) {
  overlay.openBookInfo(book, {
    categoryName: categoryName(book.categoryId),
    progress: (bookProgress(book)?.percent || 0),
  })
}

function continueRead(book) {
  router.push({ name: 'reader', params: { id: book.id } })
}

function bookProgressPercent(book) {
  return Math.round((bookProgress(book)?.percent || 0) * 100)
}

function bookProgress(book) {
  return reader.progressByBook[book.id] || book.progress
}

function compareByReadingOrder(a, b) {
  const aProgress = bookProgress(a)
  const bProgress = bookProgress(b)
  const aReadAt = new Date(aProgress?.updatedAt || 0).getTime()
  const bReadAt = new Date(bProgress?.updatedAt || 0).getTime()
  if (aReadAt !== bReadAt) return bReadAt - aReadAt
  return new Date(b.updatedAt || 0).getTime() - new Date(a.updatedAt || 0).getTime()
}

function categoryName(id) {
  if (!id) return '未分组'
  return bookshelf.categories.find(category => String(category.id) === String(id))?.name || '未分组'
}

function coverInitial(book) {
  return (book.title || '?').slice(0, 1)
}

function coverStyle(book) {
  if (book.coverUrl) {
    return { backgroundImage: `url(${book.coverUrl})`, backgroundSize: 'cover', backgroundPosition: 'center', color: 'transparent' }
  }
  const palettes = [
    ['#2f6f6d', '#d9ece7'],
    ['#9c5b34', '#f2decf'],
    ['#5a4f8f', '#dedaf1'],
    ['#406c3d', '#dfead9'],
  ]
  const [fg, bg] = palettes[Number(book.id || 1) % palettes.length]
  return { color: fg, background: bg }
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.shelf-page,
.shelf-main,
.manage-list {
  display: grid;
  gap: 16px;
}

.shelf-head,
.recent-panel,
.shelf-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
}

.empty-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.eyebrow {
  margin: 0 0 4px;
  color: var(--app-primary);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0;
  text-transform: uppercase;
}

.shelf-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.stat-panel {
  display: grid;
  gap: 6px;
  padding: 16px;
}

.stat-panel span,
.recent-main span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.stat-panel strong {
  font-size: 26px;
}

.recent-panel {
  padding: 14px;
}

.recent-cover,
.book-cover,
.list-cover {
  display: grid;
  place-items: center;
  font-weight: 900;
}

.recent-cover {
  width: 54px;
  height: 72px;
  border-radius: 5px;
  font-size: 24px;
}

.recent-main {
  min-width: 0;
  flex: 1;
}

.recent-main h2,
.recent-main p,
.book-body p {
  margin: 0;
}

.recent-main h2 {
  margin: 3px 0;
  overflow: hidden;
  font-size: 18px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-main p,
.book-body p,
.book-meta,
.list-main small,
.list-latest {
  color: var(--app-text-muted);
  font-size: 13px;
}

.shelf-layout {
  display: grid;
  grid-template-columns: 232px minmax(0, 1fr);
  gap: 16px;
}

.group-panel {
  position: sticky;
  top: 28px;
  display: grid;
  align-content: start;
  gap: 8px;
  padding: 12px;
}

.group-title,
.group-create,
.book-title-row,
.book-meta,
.book-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.book-progress {
  margin-top: 2px;
}

.group-create {
  align-items: stretch;
}

.group-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  border-radius: var(--app-radius-sm);
}

.group-row:hover,
.group-row.active {
  color: var(--app-primary-strong);
  background: var(--app-primary-soft);
}

.group-item {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  padding: 10px 11px;
  color: var(--app-text-muted);
  background: transparent;
  border: 0;
  border-radius: var(--app-radius-sm);
  cursor: pointer;
  text-align: left;
}

.group-actions {
  display: inline-flex;
  gap: 2px;
  padding-right: 5px;
}

.group-actions button {
  display: grid;
  width: 26px;
  height: 26px;
  place-items: center;
  color: var(--app-text-muted);
  background: transparent;
  border: 0;
  border-radius: 5px;
  cursor: pointer;
}

.group-actions button:hover {
  color: var(--app-primary-strong);
  background: rgba(255, 255, 255, 0.55);
}

.group-note {
  margin-top: 8px;
}

.shelf-toolbar {
  padding: 12px;
}

.shelf-toolbar .el-input {
  flex: 1;
}

.book-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(218px, 1fr));
  gap: 14px;
}

.book-card {
  display: grid;
  grid-template-rows: auto 1fr auto;
  min-height: 282px;
  overflow: hidden;
  cursor: pointer;
  transition: transform 140ms ease, box-shadow 140ms ease, border-color 140ms ease;
}

.book-card:hover {
  border-color: var(--app-border-strong);
  box-shadow: var(--app-shadow-md);
  transform: translateY(-2px);
}

.skeleton-card {
  padding: 16px;
}

.book-cover {
  height: 126px;
  color: var(--app-primary);
  background: var(--app-primary-soft);
  font-size: 42px;
}

.book-body {
  display: grid;
  align-content: start;
  gap: 8px;
  min-width: 0;
  padding: 14px 14px 10px;
}

.book-title-row h2 {
  display: -webkit-box;
  min-width: 0;
  margin: 0;
  overflow: hidden;
  font-size: 16px;
  line-height: 1.35;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.book-latest,
.list-latest,
.list-main strong,
.recent-main h2 {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.book-actions {
  padding: 0 12px 12px;
}

.book-list {
  overflow: hidden;
}

.book-row,
.manage-row {
  display: grid;
  grid-template-columns: 42px minmax(140px, 1fr) minmax(120px, 0.8fr) auto;
  gap: 12px;
  align-items: center;
  padding: 12px;
  color: var(--app-text);
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--app-border);
  cursor: pointer;
  text-align: left;
}

.book-row:hover {
  background: var(--app-bg-soft);
}

.list-cover {
  width: 42px;
  height: 54px;
  border-radius: var(--app-radius-sm);
}

.list-main {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.empty-panel {
  display: grid;
  min-height: 360px;
  place-items: center;
}

.import-form {
  display: grid;
  gap: 12px;
}

.upload-icon {
  color: var(--app-primary);
  font-size: 32px;
}

.upload-text {
  color: var(--app-text-muted);
}

.manage-row {
  grid-template-columns: auto 42px minmax(0, 1fr) auto auto;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.manage-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 10px;
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.manage-toolbar .el-select {
  width: 150px;
}

.manage-row span:nth-child(3) {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.manage-row small {
  color: var(--app-text-muted);
}

@media (max-width: 980px) {
  .shelf-layout,
  .shelf-stats {
    grid-template-columns: 1fr;
  }

  .group-panel {
    position: static;
  }
}

@media (max-width: 700px) {
  .shelf-page {
    gap: 10px;
  }

  .shelf-head,
  .recent-panel,
  .shelf-toolbar {
    display: grid;
  }

  .shelf-head {
    order: 2;
    gap: 4px;
  }

  .shelf-head .app-page-subtitle,
  .shelf-stats {
    display: none;
  }

  .shelf-head .app-page-title {
    font-size: 22px;
  }

  .eyebrow {
    display: none;
  }

  .recent-panel {
    order: 1;
    grid-template-columns: 44px minmax(0, 1fr) auto;
    gap: 10px;
    padding: 10px;
    border-radius: 0;
  }

  .recent-cover {
    width: 44px;
    height: 58px;
    font-size: 20px;
  }

  .recent-main span {
    font-size: 11px;
  }

  .recent-main h2 {
    font-size: 16px;
  }

  .recent-main p {
    font-size: 12px;
  }

  .recent-panel .el-button {
    padding: 8px 10px;
  }

  .shelf-layout {
    order: 3;
    gap: 10px;
  }

  .group-panel {
    display: flex;
    gap: 6px;
    overflow-x: auto;
    padding: 0;
    background: transparent;
    border: 0;
    box-shadow: none;
    scrollbar-width: none;
  }

  .group-title,
  .group-create,
  .group-actions,
  .group-note {
    display: none;
  }

  .group-row {
    flex: 0 0 auto;
  }

  .group-item {
    gap: 8px;
    padding: 8px 10px;
    background: var(--app-surface);
    border: 1px solid var(--app-border);
    border-radius: 999px;
    white-space: nowrap;
  }

  .shelf-toolbar {
    gap: 8px;
    padding: 0;
    background: transparent;
    border: 0;
    box-shadow: none;
  }

  .shelf-toolbar .el-button {
    display: none;
  }

  .book-grid {
    grid-template-columns: 1fr;
    gap: 10px;
  }

  .book-card {
    grid-template-columns: 72px minmax(0, 1fr);
    grid-template-rows: auto auto;
    min-height: 0;
  }

  .book-cover {
    grid-row: 1 / span 2;
    height: 100%;
    min-height: 108px;
    font-size: 28px;
  }

  .book-body {
    padding: 10px 10px 6px;
  }

  .book-actions {
    justify-content: flex-start;
    padding: 0 10px 10px;
  }

  .book-row {
    grid-template-columns: 38px minmax(0, 1fr) auto;
  }

  .list-latest {
    display: none;
  }
}
</style>
