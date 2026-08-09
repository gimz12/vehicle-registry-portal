import pluginVue from 'eslint-plugin-vue'
import vueScoped from 'eslint-plugin-vue-scoped-css'
// @ts-check
import withNuxt from './.nuxt/eslint.config.mjs'

export default withNuxt([
  ...pluginVue.configs['flat/recommended'],
  {
    plugins: {
      'vue': pluginVue,
      'vue-scoped-css': vueScoped,
    },
    rules: {
      'max-len': ['error', {
        code: 120,
        tabWidth: 2,
        ignoreUrls: true,
        ignoreStrings: true,
        ignoreTemplateLiterals: false,
        ignoreRegExpLiterals: true,
        ignoreComments: false,
      }],
      'sort-imports': ['error', {
        ignoreCase: false,
        ignoreDeclarationSort: true,
        ignoreMemberSort: false,
        memberSyntaxSortOrder: ['none', 'all', 'multiple', 'single'],
        allowSeparatedGroups: false,
      }],
      'vue/block-lang': ['error', {
        script: {
          lang: 'ts',
          allowNoLang: false,
        },
      }],
      'vue/block-order': ['error', {
        order: [
          'script[setup][lang="ts"]',
          'template',
          'style[scoped]',
        ],
      }],
      'vue/multi-word-component-names': 'off',
      'vue-scoped-css/enforce-style-type': ['error', { allows: ['scoped'] }],
      '@typescript-eslint/no-empty-object-type': ['error', {
        allowInterfaces: 'always',
      }],
    },
  },
])
