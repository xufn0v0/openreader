<template>
  <section class="book-info-shared">
    <BookCover :book="book" />
    <div class="book-info-main">
      <div class="book-info-title">
        <h2>{{ book?.title || '未命名书籍' }}</h2>
        <el-tag v-if="statusLabel" size="small" effect="plain" :type="statusType">{{ statusLabel }}</el-tag>
      </div>
      <p class="book-info-meta">
        {{ book?.author || '未知作者' }}
        <template v-if="sourceName"> · {{ sourceName }}</template>
        <template v-if="categoryName"> · {{ categoryName }}</template>
      </p>
      <p class="book-info-intro">{{ book?.intro || '暂无简介' }}</p>
      <dl class="book-info-facts">
        <div>
          <dt>最新章节</dt>
          <dd>{{ book?.lastChapter || '-' }}</dd>
        </div>
        <div>
          <dt>章节</dt>
          <dd>{{ chapterCount }}</dd>
        </div>
        <div>
          <dt>进度</dt>
          <dd>{{ progressLabel }}</dd>
        </div>
      </dl>
      <slot />
    </div>
  </section>
</template>

<script setup>
import { computed } from 'vue'
import BookCover from './BookCover.vue'

const props = defineProps({
  book: {
    type: Object,
    default: () => ({}),
  },
  sourceName: {
    type: String,
    default: '',
  },
  categoryName: {
    type: String,
    default: '',
  },
  progress: {
    type: Number,
    default: 0,
  },
  chapters: {
    type: [Array, Number],
    default: 0,
  },
  statusLabel: {
    type: String,
    default: '',
  },
  statusType: {
    type: String,
    default: 'info',
  },
})

const chapterCount = computed(() => Array.isArray(props.chapters) ? props.chapters.length : (props.chapters || props.book?.chapterCount || 0))
const progressLabel = computed(() => `${Math.round(Math.max(0, Math.min(1, props.progress || 0)) * 100)}%`)
</script>

<style scoped>
.book-info-shared {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.book-info-main {
  display: grid;
  min-width: 0;
  gap: 10px;
}

.book-info-title {
  display: flex;
  align-items: start;
  justify-content: space-between;
  gap: 10px;
}

.book-info-title h2,
.book-info-meta,
.book-info-intro,
.book-info-facts {
  margin: 0;
}

.book-info-title h2 {
  min-width: 0;
  font-size: 21px;
  line-height: 1.25;
}

.book-info-meta,
.book-info-intro,
.book-info-facts dt {
  color: var(--app-text-muted);
}

.book-info-intro {
  display: -webkit-box;
  overflow: hidden;
  line-height: 1.7;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 4;
}

.book-info-facts {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.book-info-facts div {
  display: grid;
  gap: 3px;
  min-width: 0;
  padding: 8px;
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.book-info-facts dt {
  font-size: 12px;
}

.book-info-facts dd {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  color: var(--app-text);
  font-size: 13px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 560px) {
  .book-info-shared {
    grid-template-columns: 1fr;
  }

  .book-info-facts {
    grid-template-columns: 1fr;
  }
}
</style>
