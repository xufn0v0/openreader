<template>
  <BookInfoDialog
    v-model="overlay.bookInfoVisible"
    :book="overlay.bookInfoBook"
    :source-name="overlay.bookInfoOptions.sourceName"
    :category-name="bookInfoCategory"
    :progress="bookInfoProgress"
    :chapters="overlay.bookInfoBook?.chapterCount || 0"
    :status-label="overlay.bookInfoOptions.statusLabel || sourceStatusLabel"
    :status-type="overlay.bookInfoOptions.statusType || 'info'"
  >
    <div v-if="overlay.bookInfoOptions.actions?.length" class="overlay-actions">
      <el-button
        v-for="action in overlay.bookInfoOptions.actions"
        :key="action.label"
        :type="action.type || 'default'"
        :plain="action.plain"
        :loading="!!action.loading"
        :disabled="!!action.disabled"
        @click="action.handler?.(overlay.bookInfoBook)"
      >
        {{ action.label }}
      </el-button>
    </div>
    <div v-else-if="overlay.bookInfoBook?.id" class="overlay-actions">
      <el-button type="primary" @click="continueRead(overlay.bookInfoBook)">继续阅读</el-button>
      <el-button plain @click="goDetail(overlay.bookInfoBook)">详情</el-button>
      <el-button plain :loading="loadingUpdates" @click="refreshShelf">刷新书架</el-button>
    </div>
  </BookInfoDialog>

  <el-drawer v-model="overlay.bookManageVisible" title="书籍管理" direction="rtl" size="420px" class="global-manage-drawer">
    <el-alert
      type="info"
      :closable="false"
      show-icon
      title="对齐上游 BookManage：批量分组、缓存、清缓存、导出和删除。"
    />
    <div class="manage-toolbar">
      <el-checkbox
        :model-value="allSelected"
        :indeterminate="someSelected"
        @change="toggleAll"
      >
        全选
      </el-checkbox>
      <el-select v-model="batchCategoryId" placeholder="批量分组" clearable size="small">
        <el-option label="未分组" value="" />
        <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
      </el-select>
      <el-button size="small" :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchSetCategory">设置分组</el-button>
      <el-button size="small" :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchCacheBooks">缓存</el-button>
      <el-button size="small" :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchClearCache">清缓存</el-button>
      <el-button size="small" :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchExportBooks">导出</el-button>
      <el-button size="small" type="danger" plain :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchDeleteBooks">删除</el-button>
    </div>
    <div class="manage-list">
      <div v-for="book in managedBooks" :key="book.id" class="manage-row">
        <el-checkbox v-model="selectedBookIds" :value="book.id" />
        <BookCover :book="book" size="small" />
        <span class="manage-book-main">
          <strong>{{ book.title }}</strong>
          <small>{{ categoryName(book.categoryId) }} · {{ book.chapterCount || 0 }} 章 · {{ progressLabel(book) }}</small>
        </span>
        <el-button size="small" @click="overlay.openBookInfo(book)">信息</el-button>
        <el-button size="small" type="danger" plain @click="deleteBook(book)">删除</el-button>
      </div>
    </div>
  </el-drawer>

  <el-drawer v-model="overlay.bookGroupVisible" title="分组管理" direction="rtl" size="360px">
    <div class="group-create">
      <el-input v-model="newGroupName" placeholder="新增分组" size="small" @keyup.enter="createCategory" />
      <el-button size="small" type="primary" :disabled="!newGroupName.trim()" @click="createCategory">新增</el-button>
    </div>
    <div class="group-list">
      <div v-for="category in bookshelf.categories" :key="category.id" class="group-row">
        <span>{{ category.name }}</span>
        <span class="group-actions">
          <el-button size="small" text @click="moveGroup(category, -1)">上移</el-button>
          <el-button size="small" text @click="moveGroup(category, 1)">下移</el-button>
          <el-button size="small" text @click="renameGroup(category)">重命名</el-button>
          <el-button size="small" text type="danger" @click="deleteGroup(category)">删除</el-button>
        </span>
      </div>
    </div>
    <el-empty v-if="!bookshelf.categories.length" description="还没有自定义分组" />
  </el-drawer>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { checkBookUpdates } from '../api/books'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import BookCover from './BookCover.vue'
import BookInfoDialog from './BookInfoDialog.vue'

const router = useRouter()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()

const selectedBookIds = ref([])
const batchCategoryId = ref('')
const batchBusy = ref(false)
const loadingUpdates = ref(false)
const newGroupName = ref('')

const allSelected = computed(() => bookshelf.books.length > 0 && selectedBookIds.value.length === bookshelf.books.length)
const someSelected = computed(() => selectedBookIds.value.length > 0 && selectedBookIds.value.length < bookshelf.books.length)
const bookInfoCategory = computed(() => overlay.bookInfoOptions.categoryName || categoryName(overlay.bookInfoBook?.categoryId))
const bookInfoProgress = computed(() => {
  const book = overlay.bookInfoBook
  return book ? (reader.progressByBook[book.id]?.percent || book.progress?.percent || 0) : 0
})
const sourceStatusLabel = computed(() => overlay.bookInfoBook?.sourceId ? '远程书籍' : '本地书籍')
const managedBooks = computed(() => [...bookshelf.books].sort(compareByReadingOrder))

watch(
  () => overlay.bookManageVisible || overlay.bookGroupVisible,
  async (visible) => {
    if (!visible) return
    try {
      await Promise.all([bookshelf.loadCategories(), bookshelf.loadBooks()])
    } catch (err) {
      ElMessage.error(readError(err, '加载书架数据失败'))
    }
  },
)

function categoryName(id) {
  if (!id) return '未分组'
  return bookshelf.categories.find(category => String(category.id) === String(id))?.name || '未分组'
}

function progressLabel(book) {
  const progress = reader.progressByBook[book.id] || book.progress
  return `${Math.round((progress?.percent || 0) * 100)}%`
}

function compareByReadingOrder(a, b) {
  const aProgress = reader.progressByBook[a.id] || a.progress
  const bProgress = reader.progressByBook[b.id] || b.progress
  const aReadAt = new Date(aProgress?.updatedAt || 0).getTime()
  const bReadAt = new Date(bProgress?.updatedAt || 0).getTime()
  if (aReadAt !== bReadAt) return bReadAt - aReadAt
  return new Date(b.updatedAt || 0).getTime() - new Date(a.updatedAt || 0).getTime()
}

function toggleAll(value) {
  selectedBookIds.value = value ? bookshelf.books.map(book => book.id) : []
}

function continueRead(book) {
  overlay.closeBookInfo()
  router.push({ name: 'reader', params: { id: book.id } })
}

function goDetail(book) {
  overlay.closeBookInfo()
  router.push({ name: 'book-detail', params: { id: book.id } })
}

async function refreshShelf() {
  loadingUpdates.value = true
  try {
    const { data } = await checkBookUpdates()
    await bookshelf.loadBooks()
    ElMessage.success(data?.newChapters ? `发现 ${data.newChapters} 个新章节` : '暂未发现新章节')
  } catch (err) {
    ElMessage.error(readError(err, '刷新失败'))
  } finally {
    loadingUpdates.value = false
  }
}

async function batchSetCategory() {
  if (!selectedBookIds.value.length) return
  batchBusy.value = true
  try {
    const categoryId = batchCategoryId.value ? Number(batchCategoryId.value) : null
    await bookshelf.batchSetCategory([...selectedBookIds.value], categoryId)
    selectedBookIds.value = []
    ElMessage.success('分组已更新')
  } catch (err) {
    ElMessage.error(readError(err, '批量分组失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchCacheBooks() {
  if (!selectedBookIds.value.length) return
  batchBusy.value = true
  try {
    const data = await bookshelf.batchCacheBooks([...selectedBookIds.value])
    ElMessage.success(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '批量缓存失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchClearCache() {
  if (!selectedBookIds.value.length) return
  try {
    await ElMessageBox.confirm(`确定清理选中 ${selectedBookIds.value.length} 本书的章节缓存吗？`, '清理缓存', { type: 'warning' })
    batchBusy.value = true
    const data = await bookshelf.batchClearCache([...selectedBookIds.value])
    ElMessage.success(`已清理 ${data.cleared || 0} 个章节缓存`)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchExportBooks() {
  if (!selectedBookIds.value.length) return
  batchBusy.value = true
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
    batchBusy.value = false
  }
}

async function batchDeleteBooks() {
  if (!selectedBookIds.value.length) return
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${selectedBookIds.value.length} 本书吗？`, '批量删除', { type: 'warning' })
    batchBusy.value = true
    await bookshelf.batchDeleteBooks([...selectedBookIds.value])
    selectedBookIds.value = []
    ElMessage.success('已批量删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '批量删除失败'))
  } finally {
    batchBusy.value = false
  }
}

async function deleteBook(book) {
  try {
    await ElMessageBox.confirm(`确定删除《${book.title}》吗？`, '删除书籍', { type: 'warning' })
    await bookshelf.removeBook(book.id)
    ElMessage.success('书籍已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除失败'))
  }
}

async function createCategory() {
  const name = newGroupName.value.trim()
  if (!name) return
  try {
    await bookshelf.addCategory({ name })
    newGroupName.value = ''
    ElMessage.success('分组已创建')
  } catch (err) {
    ElMessage.error(readError(err, '创建分组失败'))
  }
}

async function renameGroup(category) {
  try {
    const { value } = await ElMessageBox.prompt('输入新的分组名称', '重命名分组', {
      inputValue: category.name,
      inputValidator: value => !!value?.trim() || '分组名称不能为空',
    })
    const name = value.trim()
    if (!name || name === category.name) return
    await bookshelf.renameCategory(category.id, { name })
    ElMessage.success('分组已重命名')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '重命名失败'))
  }
}

async function deleteGroup(category) {
  try {
    await ElMessageBox.confirm(`确定删除分组“${category.name}”吗？`, '删除分组', { type: 'warning' })
    await bookshelf.removeCategory(category.id)
    ElMessage.success('分组已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除分组失败'))
  }
}

async function moveGroup(category, direction) {
  const categories = [...bookshelf.categories]
  const index = categories.findIndex(item => item.id === category.id)
  const targetIndex = index + direction
  if (index < 0 || targetIndex < 0 || targetIndex >= categories.length) return
  const [moved] = categories.splice(index, 1)
  categories.splice(targetIndex, 0, moved)
  try {
    await bookshelf.reorderCategoryIds(categories.map(item => item.id))
    ElMessage.success('分组排序已更新')
  } catch (err) {
    ElMessage.error(readError(err, '分组排序失败'))
  }
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.overlay-actions,
.manage-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.overlay-actions {
  margin-top: 4px;
}

.manage-toolbar {
  margin: 12px 0;
  padding: 10px;
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.manage-toolbar .el-select {
  width: 150px;
}

.manage-list,
.group-list {
  display: grid;
  gap: 10px;
}

.manage-row,
.group-row,
.group-create {
  display: grid;
  align-items: center;
  gap: 10px;
}

.manage-row {
  grid-template-columns: auto 44px minmax(0, 1fr) auto auto;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.manage-book-main {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.manage-book-main strong,
.manage-book-main small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.manage-book-main small {
  color: var(--app-text-muted);
}

.group-create {
  grid-template-columns: minmax(0, 1fr) auto;
  margin-bottom: 12px;
}

.group-row {
  grid-template-columns: minmax(0, 1fr) auto;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.group-actions {
  display: inline-flex;
  flex-wrap: wrap;
  justify-content: flex-end;
}

@media (max-width: 560px) {
  .manage-row {
    grid-template-columns: auto 42px minmax(0, 1fr);
  }

  .manage-row .el-button {
    grid-column: span 3;
  }
}
</style>
