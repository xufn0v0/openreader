import { onBeforeUnmount, onMounted } from 'vue'

const SWIPE_THRESHOLD = 60
const EDGE_WIDTH = 80
const PINCH_DEAD_ZONE = 40

export function useGesture(elementRef, handlers) {
  const {
    onSwipeLeft, onSwipeRight,
    onCenterTap, onEdgeLeftTap, onEdgeRightTap,
    onPinchIn, onPinchOut,
  } = handlers

  let startX = 0
  let startY = 0
  let startTime = 0
  let startDistance = 0
  let cleanup = null

  function getDistance(touches) {
    if (touches.length < 2) return 0
    const dx = touches[0].clientX - touches[1].clientX
    const dy = touches[0].clientY - touches[1].clientY
    return Math.sqrt(dx * dx + dy * dy)
  }

  function handleTouchStart(event) {
    if (event.touches.length === 2) {
      startDistance = getDistance(event.touches)
      return
    }
    if (event.touches.length !== 1) return
    startX = event.touches[0].clientX
    startY = event.touches[0].clientY
    startTime = Date.now()
  }

  function handleTouchEnd(event) {
    if (event.touches.length > 0) return

    const endX = event.changedTouches[0].clientX
    const endY = event.changedTouches[0].clientY
    const dx = endX - startX
    const dy = endY - startY
    const elapsed = Date.now() - startTime

    if (Math.abs(dx) < 5 && Math.abs(dy) < 5 && elapsed < 300) {
      const el = elementRef.value
      if (!el) return
      const rect = el.getBoundingClientRect()
      const relX = endX - rect.left

      if (relX < EDGE_WIDTH) {
        onEdgeLeftTap?.()
        return
      }
      if (relX > rect.width - EDGE_WIDTH) {
        onEdgeRightTap?.()
        return
      }
      onCenterTap?.()
      return
    }

    if (Math.abs(dx) > Math.abs(dy) && Math.abs(dx) > SWIPE_THRESHOLD) {
      if (dx < 0) {
        onSwipeLeft?.()
      } else {
        onSwipeRight?.()
      }
    }
  }

  function handleTouchMove(event) {
    if (event.touches.length === 2 && startDistance > 0) {
      const currentDistance = getDistance(event.touches)
      const delta = currentDistance - startDistance
      if (delta > PINCH_DEAD_ZONE) {
        onPinchOut?.()
        startDistance = 0
      } else if (delta < -PINCH_DEAD_ZONE) {
        onPinchIn?.()
        startDistance = 0
      }
    }
  }

  onMounted(() => {
    const el = elementRef.value
    if (!el) return
    el.addEventListener('touchstart', handleTouchStart, { passive: true })
    el.addEventListener('touchend', handleTouchEnd, { passive: true })
    el.addEventListener('touchmove', handleTouchMove, { passive: true })
    cleanup = () => {
      el.removeEventListener('touchstart', handleTouchStart)
      el.removeEventListener('touchend', handleTouchEnd)
      el.removeEventListener('touchmove', handleTouchMove)
    }
  })

  onBeforeUnmount(() => {
    cleanup?.()
  })
}
