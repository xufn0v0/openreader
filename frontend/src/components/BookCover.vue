<template>
  <span class="book-cover-shared" :class="`size-${size}`" :style="coverStyle">{{ initial }}</span>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  book: {
    type: Object,
    default: () => ({}),
  },
  size: {
    type: String,
    default: 'md',
  },
})

const initial = computed(() => (props.book?.title || props.book?.name || '?').slice(0, 1))

const coverStyle = computed(() => {
  if (props.book?.coverUrl) {
    return {
      backgroundImage: `url(${props.book.coverUrl})`,
      backgroundPosition: 'center',
      backgroundSize: 'cover',
      color: 'transparent',
    }
  }
  const palettes = [
    ['#2f6f6d', '#d9ece7'],
    ['#9c5b34', '#f2decf'],
    ['#5a4f8f', '#dedaf1'],
    ['#406c3d', '#dfead9'],
  ]
  const seed = Number(props.book?.id || props.book?.sourceId || props.book?.title?.length || 1)
  const [color, background] = palettes[seed % palettes.length]
  return { color, background }
})
</script>

<style scoped>
.book-cover-shared {
  display: grid;
  width: 72px;
  height: 96px;
  place-items: center;
  flex: 0 0 auto;
  border-radius: 5px;
  box-shadow: 0 10px 24px rgba(58, 41, 10, 0.12);
  font-size: 30px;
  font-weight: 900;
  line-height: 1;
}

.book-cover-shared.size-small {
  width: 44px;
  height: 58px;
  font-size: 20px;
}
</style>
