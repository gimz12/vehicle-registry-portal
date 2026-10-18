<script setup lang="ts">
import TopNav from '~/components/TopNav.vue'
</script>

<template>
  <div class="flex min-h-screen flex-col bg-paper text-ink">
    <TopNav />
    <main class="mx-auto w-full max-w-6xl flex-1 px-5 py-8 sm:py-10">
      <slot />
    </main>
    <footer class="border-t border-line px-5 py-5">
      <div class="mx-auto flex w-full max-w-6xl flex-wrap justify-between gap-2 text-xs text-muted">
        <span>Research prototype. Synthetic data only; no real person or vehicle is described.</span>
        <span>Owner identities stay off the ledger. The registry declares the checks it could not make.</span>
      </div>
    </footer>
  </div>
</template>
