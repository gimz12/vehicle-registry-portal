<template>
  <header class="border-b-4 border-accent bg-panel">
    <div class="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-x-8 gap-y-2 px-5 py-3">
      <NuxtLink
        to="/"
        class="flex items-center gap-3"
      >
        <span
          aria-hidden="true"
          class="grid size-8 place-items-center rounded-sm bg-accent font-semibold text-accent-ink"
        >VR</span>
        <span class="leading-tight">
          <span class="block font-semibold">National Vehicle Registry</span>
          <span class="block text-xs text-muted">Pre-purchase disclosure, Sri Lanka</span>
        </span>
      </NuxtLink>
      <nav
        class="flex items-center gap-5 text-sm"
        aria-label="Sections"
      >
        <NuxtLink
          to="/"
          class="hover:text-accent"
          active-class="font-semibold text-accent"
        >
          Check a vehicle
        </NuxtLink>
        <NuxtLink
          to="/desk"
          class="hover:text-accent"
          active-class="font-semibold text-accent"
        >
          Agency desk
        </NuxtLink>
        <NuxtLink
          to="/results"
          class="hover:text-accent"
          active-class="font-semibold text-accent"
        >
          Experiment results
        </NuxtLink>
        <NuxtLink
          to="/live"
          class="hover:text-accent"
          active-class="font-semibold text-accent"
        >
          Live network
        </NuxtLink>
      </nav>
    </div>
  </header>
</template>
