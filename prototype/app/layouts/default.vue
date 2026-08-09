<script setup lang="ts">
import TopNav from '~/components/TopNav.vue'
</script>

<template>
  <div class="flex min-h-screen flex-col bg-paper text-ink">
    <TopNav />
    <main class="mx-auto w-full max-w-4xl flex-1 px-5 py-10">
      <slot />
    </main>
    <footer class="border-t border-line px-5 py-4 text-center text-xs text-muted">
      Research prototype — privacy-scoped blockchain vehicle registry for Sri Lanka
    </footer>
  </div>
</template>
