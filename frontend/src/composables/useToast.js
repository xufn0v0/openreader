import { ref } from 'vue'

const toasts = ref([])
let nextId = 0

export function useToast() {
  function show(message, type = 'info', duration = 2500) {
    const id = nextId++
    toasts.value.push({ id, message, type })
    if (duration > 0) {
      setTimeout(() => {
        toasts.value = toasts.value.filter(t => t.id !== id)
      }, duration)
    }
  }

  function success(msg) { show(msg, 'success') }
  function error(msg) { show(msg, 'error', 4000) }
  function info(msg) { show(msg, 'info') }

  return { toasts, show, success, error, info }
}
