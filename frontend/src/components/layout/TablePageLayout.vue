<template>
  <div class="desk-board ops-desk" :class="{ 'is-mobile': isMobile }">
    <section v-if="$slots.intro" class="desk-board__intro ops-desk__intro" aria-label="page intro">
      <slot name="intro" />
    </section>

    <div class="ops-console" :class="{ 'is-mobile': isMobile, 'has-intro': !!$slots.intro }">
      <div v-if="$slots.actions" class="ops-console__actions">
        <div class="ops-console__actions-inner">
          <slot name="actions" />
        </div>
      </div>

      <div v-if="$slots.filters" class="ops-console__filters">
        <div class="ops-console__filters-body">
          <slot name="filters" />
        </div>
      </div>

      <div class="ops-console__surface">
        <div class="ops-console__table-frame">
          <div v-if="$slots['before-table']" class="ops-console__before-table">
            <slot name="before-table" />
          </div>
          <div class="ops-console__table-scroll">
            <slot name="table" />
          </div>
        </div>
      </div>

      <div v-if="$slots.pagination" class="ops-console__pager">
        <slot name="pagination" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

const isMobile = ref(false)

const checkMobile = () => {
  isMobile.value = window.innerWidth < 1024
}

onMounted(() => {
  checkMobile()
  window.addEventListener('resize', checkMobile)
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})
</script>

<style scoped>
.ops-desk {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.ops-desk__intro {
  margin-bottom: 0;
}

.ops-console {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: calc(100vh - 64px - 3.5rem);
  min-height: 420px;
}

.ops-console.has-intro {
  height: calc(100vh - 64px - 3.5rem - 108px);
}

.ops-console__actions {
  flex-shrink: 0;
}

.ops-console__actions-inner {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
}

.ops-console__filters {
  flex-shrink: 0;
  border: 1px solid rgba(11, 18, 32, 0.1);
  border-radius: 14px;
  overflow: hidden;
  background: linear-gradient(180deg, #ffffff, #eef2f8);
  box-shadow: 0 1px 0 rgba(255,255,255,0.9) inset, 0 14px 28px -24px rgba(11,18,32,0.32);
}

:global(.dark) .ops-console__filters {
  border-color: rgba(148, 163, 184, 0.14);
  background: linear-gradient(180deg, rgba(28, 36, 48, 0.9), rgba(18, 24, 32, 0.94));
  box-shadow: none;
}

.ops-console__filters-body {
  padding: 12px 14px;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}

.ops-console__surface {
  flex: 1;
  min-height: 0;
  display: flex;
}

.ops-console__table-frame {
  flex: 1;
  min-height: min(420px, 48vh);
  border-radius: 16px;
  border: 1px solid rgba(11, 18, 32, 0.1);
  background: #fff;
  box-shadow: 0 1px 0 rgba(255,255,255,0.8) inset, 0 22px 42px -30px rgba(11,18,32,0.42);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.ops-console__before-table {
  flex-shrink: 0;
  padding: 10px 12px 0;
}

:global(.dark) .ops-console__table-frame {
  border-color: rgba(148, 163, 184, 0.14);
  background: rgba(14, 20, 30, 0.92);
  box-shadow: none;
}

.ops-console__table-scroll {
  flex: 1 1 auto;
  min-height: 280px;
  overflow: auto;
  display: flex;
  flex-direction: column;
}

.ops-console__pager {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  padding: 6px 4px;
  border-radius: 12px;
  background: linear-gradient(180deg, rgba(255,255,255,0.7), rgba(238,242,248,0.9));
  border: 1px solid rgba(11, 18, 32, 0.06);
}

:global(.dark) .ops-console__pager {
  background: rgba(18, 24, 32, 0.55);
  border-color: rgba(148, 163, 184, 0.1);
}

.ops-console.is-mobile {
  height: auto;
  min-height: 0;
}

.ops-console.is-mobile.has-intro {
  height: auto;
}
</style>

