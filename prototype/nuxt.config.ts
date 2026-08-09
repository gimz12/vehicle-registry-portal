import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  modules: [
    '@nuxt/eslint',
    '@nuxt/test-utils/module',
  ],

  $development: {
    runtimeConfig: {
      logger: {
        level: 'debug',
      },
      public: {},
    },
    sourcemap: {
      server: true,
      client: true,
    },
  },

  devtools: {
    timeline: {
      enabled: true,
    },
  },

  app: {
    head: {
      title: 'National Vehicle Registry — Pre-Purchase Disclosure',
    },
  },

  css: [
    '~/assets/css/main.css',
  ],

  runtimeConfig: {
    logger: {
      level: 'info',
    },
    public: {},
  },

  sourcemap: {
    server: false,
    client: false,
  },

  future: {
    compatibilityVersion: 4,
  },

  experimental: {
    typedPages: true,
  },

  compatibilityDate: '2025-06-01',

  vite: {
    plugins: [
      tailwindcss(),
    ],
    define: {
      // Explicitly set `import.meta.test` for both client and server builds.
      // Detection of the test mode is based on the `std-env` package,
      // which is used by Nuxt for the same.
      'import.meta.test': process.env.NODE_ENV === 'test',
    },
  },

  typescript: {
    // Build-time checking is off because vite-plugin-checker splits the tsconfig
    // path at the space in the parent directory name ("RN BOL").
    // `yarn lint` / `yarn ci:lint` still run `nuxt typecheck` standalone.
    typeCheck: false,
  },

  telemetry: false,

  eslint: {
    config: {
      stylistic: true,
    },
  },
})
