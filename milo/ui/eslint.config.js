// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// @ts-check

import prettierConfig from 'eslint-config-prettier';
import importPlugin from 'eslint-plugin-import';
import jestPlugin from 'eslint-plugin-jest';
import jsxA11yPlugin from 'eslint-plugin-jsx-a11y';
import prettierPlugin from 'eslint-plugin-prettier';
import reactPlugin from 'eslint-plugin-react';
// eslint-disable-next-line import/default
import hooksPlugin from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import globals from 'globals';
import tseslint from 'typescript-eslint';

// Export a plain array (the new recommended way)
export default [
  // 1. Global Ignores (from .eslintignore)
  {
    ignores: ['**/node_modules', 'generated/', 'dist/', 'src/proto'],
  },

  // 2. TypeScript-specific configuration (from tseslint.configs.recommended)
  // We spread this at the top level.
  ...tseslint.configs.recommended,

  // 3. Main Configuration (Applied to all files)
  {
    files: ['**/*.{js,mjs,cjs,ts,jsx,tsx}'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      // Manually add the parser (which tseslint.config() did automatically)
      parser: tseslint.parser,
      globals: {
        ...globals.browser,
        ...globals.es2021,
      },
    },
    plugins: {
      // Manually add the plugin (which tseslint.config() did automatically)
      '@typescript-eslint': tseslint.plugin,
      react: reactPlugin,
      prettier: prettierPlugin,
      'jsx-a11y': jsxA11yPlugin,
      import: importPlugin,
      'react-refresh': reactRefresh,
      'react-hooks': hooksPlugin,
    },
    settings: {
      react: {
        version: 'detect',
      },
      'import/parsers': {
        '@typescript-eslint/parser': ['.ts', '.tsx'],
      },
      'import/resolver': {
        typescript: {},
      },
    },
    // Apply all configs.
    // We explicitly spread the rules from each config.
    rules: {
      ...reactPlugin.configs.recommended.rules,
      ...reactPlugin.configs['jsx-runtime'].rules,
      ...hooksPlugin.configs.recommended.rules,
      ...importPlugin.configs.recommended.rules,
      ...importPlugin.configs.typescript.rules,
      ...jsxA11yPlugin.flatConfigs.recommended.rules,
      ...prettierConfig.rules, // This one is fine, it has types

      // Custom rules from .eslintrc.cjs
      // Note: @typescript-eslint rules are already applied
      // from `...tseslint.configs.recommended` above.
      // We just need to add the override:
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrorsIgnorePattern: '^_',
        },
      ],
      camelcase: 'off',
      eqeqeq: ['error', 'always'],
      'import/no-duplicates': 'off',
      'import/no-unresolved': ['error', { ignore: ['^virtual:'] }],
      'import/order': [
        'error',
        {
          pathGroups: [
            {
              pattern: '@root/**',
              group: 'external',
              position: 'after',
            },
            {
              pattern: '@/**',
              group: 'external',
              position: 'after',
            },
          ],
          alphabetize: {
            order: 'asc',
            orderImportKind: 'asc',
          },
          'newlines-between': 'always',
        },
      ],
      'max-len': [
        'error',
        { code: 140, ignoreUrls: true, ignoreRegExpLiterals: true },
      ],
      'new-cap': 'off',
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['lodash-es'],
              importNames: ['chain'],
              message:
                '`chain` from `lodash-es` does not work with tree-shaking',
            },
            {
              group: ['lodash-es/chain'],
              importNames: ['default'],
              message:
                '`chain` from `lodash-es` does not work with tree-shaking',
            },
            {
              group: ['react-router'],
              importNames: ['useSearchParams'],
              message: 'use `useSyncedSearchParams` instead',
            },
            {
              group: ['lit/directives/unsafe-html.js'],
              importNames: ['unsafeHTML'],
              message: 'use `@/common/components/sanitized_html` instead',
            },
          ],
        },
      ],
      'no-console': ['error'],
      'prettier/prettier': [
        'error',
        {
          singleQuote: true,
        },
      ],
      'react/no-danger': ['error'],
      'react/no-unknown-property': ['error', { ignore: ['css'] }],
      'react-refresh/only-export-components': [
        'warn',
        { allowConstantExport: true },
      ],
      'require-jsdoc': 'off',
      'valid-jsdoc': 'off',
    },
  },

  // 4. Overrides for Test Files
  {
    files: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
    plugins: {
      jest: jestPlugin,
    },
    rules: {
      ...jestPlugin.configs.recommended.rules,
      // Test-specific rules
      '@typescript-eslint/no-non-null-assertion': 'off',
      '@typescript-eslint/no-empty-function': 'off',
      'no-restricted-imports': 'off',
      'react-refresh/only-export-components': 'off',
    },
  },
  {
    // Override for testing_tools
    files: ['**/testing_tools/**'],
    rules: {
      '@typescript-eslint/no-non-null-assertion': 'off',
      '@typescript-eslint/no-empty-function': 'off',
      'no-restricted-imports': 'off',
      'react-refresh/only-export-components': 'off',
    },
  },

  // 5. Override for Cypress Files
  {
    files: ['cypress/**/*.ts', 'cypress/**/*.tsx'],
    rules: {
      'spaced-comment': ['error', 'always', { markers: ['/'] }],
    },
  },
];
