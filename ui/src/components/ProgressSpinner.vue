<template>
  <div class="progress-spinner-container">
    <div class="progress-spinner" :style="spinnerStyle">
      <div class="spinner-segment" v-for="n in segments" :key="n" :style="getSegmentStyle(n)"></div>
    </div>
    <div v-if="showPercent" class="progress-text">{{ progressText }}</div>
  </div>
</template>

<script>
import { computed } from 'vue'

export default {
  name: 'ProgressSpinner',
  props: {
    // 进度值 (0-100)
    progress: {
      type: Number,
      default: 0
    },
    // 旋转速度 (毫秒/圈)
    speed: {
      type: Number,
      default: 1500
    },
    // 大小 (像素)
    size: {
      type: Number,
      default: 60
    },
    // 颜色
    color: {
      type: String,
      default: 'var(--primary-color)'
    },
    // 线宽
    strokeWidth: {
      type: Number,
      default: 4
    },
    // 是否显示百分比文本
    showPercent: {
      type: Boolean,
      default: true
    },
    // 分段数量
    segments: {
      type: Number,
      default: 12
    }
  },
  setup(props) {
    // 计算进度条样式
    const spinnerStyle = computed(() => ({
      width: `${props.size}px`,
      height: `${props.size}px`,
      animationDuration: `${props.speed}ms`
    }))
    
    // 获取每个分段的样式
    const getSegmentStyle = (segmentIndex) => {
      const segmentOpacity = Math.max(1 - ((props.segments - segmentIndex) / props.segments), 0.1)
      const transform = `rotate(${(segmentIndex - 1) * (360 / props.segments)}deg)`
      
      return {
        height: `${props.size / 2}px`,
        transform,
        backgroundColor: props.color,
        width: `${props.strokeWidth}px`,
        opacity: segmentOpacity
      }
    }
    
    // 进度文本
    const progressText = computed(() => {
      const roundedProgress = Math.round(props.progress)
      return `${roundedProgress}%`
    })
    
    return {
      spinnerStyle,
      getSegmentStyle,
      progressText
    }
  }
}
</script>

<style scoped>
.progress-spinner-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.progress-spinner {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  animation: rotate linear infinite;
}

.spinner-segment {
  position: absolute;
  top: 0;
  left: 50%;
  transform-origin: center bottom;
  border-radius: 4px;
}

.progress-text {
  margin-top: 8px;
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary);
}

@keyframes rotate {
  0% {
    transform: rotate(0deg);
  }
  100% {
    transform: rotate(360deg);
  }
}
</style> 