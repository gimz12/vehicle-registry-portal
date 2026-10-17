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
      title: 'National Vehicle Registry: pre-purchase disclosure',
      htmlAttrs: { lang: 'en' },
      meta: [{ name: 'color-scheme', content: 'light dark' }],
      link: [
        { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
        { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' },
        {
          rel: 'stylesheet',
          href: 'https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:wght@400;500;600;700&display=swap',
        },
      ],
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
    // NUXT_RESULTS_DIR: frozen sweep output served on /results.
    resultsDir: 'eval/results/2026-10-03',
    fabric: {
      channelName: 'registry',
      chaincodeName: 'vehicle-registry',
      // NUXT_FABRIC_ORGANIZATIONS_PATH: the peerOrganizations directory (default under ~/fabric-registry).
      organizationsPath: '',
      peerHost: 'localhost',
      // NUXT_FABRIC_CUSTODY_MODE: "services" (offchain/custody/) or "files" (offchainDir fallback).
      custodyMode: 'services',
      custodyUrls: 'dmt=http://127.0.0.1:8101,police=http://127.0.0.1:8102,customs=http://127.0.0.1:8103,'
        + 'vet=http://127.0.0.1:8104,insurer=http://127.0.0.1:8105',
      // The portal's tokens, written by offchain/custody/setup-env.sh (git-ignored).
      custodyEnvFile: 'offchain/custody/portal.env',
      // NUXT_FABRIC_OFFCHAIN_DIR: the fallback file store (files mode). Demo mode uses demoDir.
      offchainDir: 'offchain/dev-store',
      // NUXT_FABRIC_REGISTRY_KEYS="<k_v hex>:<k_own hex>"; otherwise a key file is created in offchainDir.
      registryKeys: '',
      // NUXT_FABRIC_DEMO_DIR: serve cmd/disclose output instead of Fabric.
      demoDir: '',
      // NUXT_FABRIC_OPS: the peers' and orderers' operations endpoints (Prometheus), polled by /live.
      ops: 'peer0.org1=http://localhost:9444,peer0.org2=http://localhost:9445,peer0.org3=http://localhost:9449,'
        + 'peer0.org4=http://localhost:9450,peer0.org5=http://localhost:9451,'
        + 'orderer=http://localhost:9443,orderer2=http://localhost:9446,orderer3=http://localhost:9447',
    },
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
      // Set `import.meta.test` for both client and server builds.
      'import.meta.test': process.env.NODE_ENV === 'test',
    },
  },

  typescript: {
    // Off: vite-plugin-checker splits the tsconfig path at the space in "RN BOL".
    // `yarn ci:lint` still runs `nuxt typecheck` standalone.
    typeCheck: false,
  },

  telemetry: false,

  eslint: {
    config: {
      stylistic: true,
    },
  },
})
