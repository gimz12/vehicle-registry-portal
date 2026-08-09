<script setup lang="ts">
const chassis = ref('')
</script>

<template>
  <div class="mx-auto max-w-xl py-10 text-center">
    <h1 class="text-3xl font-bold text-balance">
      Check a vehicle before you pay
    </h1>
    <p class="mt-4 text-muted">
      Enter a chassis or registration number to see the vehicle's pseudonymised history,
      fraud alerts, and the checks that could not be performed.
    </p>
    <form
      class="mt-8 flex gap-2"
      @submit.prevent="navigateTo(`/vehicle/${chassis}`)"
    >
      <input
        v-model="chassis"
        required
        placeholder="e.g. NHP10-1234567"
        aria-label="Chassis or registration number"
        class="flex-1 rounded border-2 border-line bg-panel px-3.5 py-2.5
               focus:border-accent focus:outline-none"
      >
      <button
        type="submit"
        class="rounded bg-accent px-5 py-2.5 font-semibold text-white
               hover:opacity-90 focus:outline-2 focus:outline-offset-2 focus:outline-accent"
      >
        Check vehicle
      </button>
    </form>
  </div>
</template>
