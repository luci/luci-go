// Copyright 2023 The LUCI Authors.
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

// eslint-disable-next-line no-undef
module.exports = {
  env: {
    browser: true,
    es2021: true,
  },
  plugins: ['react', '@typescript-eslint', 'prettier', 'jsx-a11y', 'import'],
  extends: [
    'eslint:recommended',
    'plugin:react/recommended',
    'plugin:react/jsx-runtime',
    'plugin:react-hooks/recommended',
    'google',
    'plugin:@typescript-eslint/recommended',
    'plugin:import/recommended',
    'plugin:import/typescript',
    'plugin:jsx-a11y/recommended',
    // "plugin:prettier/recommended" needs to be added last because it needs to
    // disable rules that are incompatible with the prettier.
    'plugin:prettier/recommended',
  ],
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
  parser: '@typescript-eslint/parser',
  parserOptions: {
    ecmaFeatures: {
      jsx: true,
    },
    ecmaVersion: 'latest',
    sourceType: 'module',
  },
  rules: {
    '@typescript-eslint/no-unused-vars': [
      'error',
      {
        // Use cases:
        // - declare a function property with a default value
        // - ignore some function parameters when writing a callback function
        // See http://b/182855639.
        argsIgnorePattern: '^_',
        // Use cases:
        // - explicitly ignore some elements from a destructed array
        // - explicitly ignore some inferred type parameters
        // See http://b/182855639.
        varsIgnorePattern: '^_',
      },
    ],

    // Code generated from protobuf may contain '_' in the identifier name,
    // (e.g. `BuilderMask_BuilderMaskType`) and therefore triggering the error.
    // `"ignoreImports": true` doesn't fix the issue because checks are still
    // applied where the imported symbol is used.
    //
    // Since this rule doesn't add a lot of value (it only checks whether there
    // are '_' in non-leading/trailing positions), disable it to reduce noise.
    //
    // Note that we should still generally use camelcase.
    camelcase: 0,

    // `==` may perform type conversion in some cases and is unintuitive.
    eqeqeq: ['error', 'always'],

    // We need to import Lit component definition file separately as a
    // side-effect (e.g. `import './a_component'`) because type-only imports
    // (e.g. `import { AComponent } from './a_component'`) may get optimized
    // away by the bundler.
    'import/no-duplicates': 0,

    // See https://vitejs.dev/guide/api-plugin.html#virtual-modules-convention.
    'import/no-unresolved': ['error', { ignore: ['^virtual:'] }],

    // Group internal dependencies together.
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

    'no-restricted-imports': [
      'error',
      {
        patterns: [
          {
            group: ['lodash-es'],
            importNames: ['chain'],
            message: '`chain` from `lodash-es` does not work with tree-shaking',
          },
          {
            group: ['lodash-es/chain'],
            importNames: ['default'],
            message: '`chain` from `lodash-es` does not work with tree-shaking',
          },
          // Ban the use of `useSearchParams` because it may accidentally
          // override search params when updater pattern is used.
          {
            group: ['react-router-dom'],
            importNames: ['useSearchParams'],
            message: 'use `useSyncedSearchParams` instead',
          },
        ],
      },
    ],

    // Generated protobuf types could be very long (e.g.
    // `ClusterResponse_ClusteredTestResult_ClusterEntry`). Set a high max-len
    // so we don't need to disable this rule whenever those long types are used.
    //
    // Note that the prettier will still try to reformat the code into 80 cols
    // when appropriate. And we should generally fit our code in 80 cols too.
    'max-len': [
      'error',
      { code: 140, ignoreUrls: true, ignoreRegExpLiterals: true },
    ],

    // Ban `console.log` to encourage displaying message via DOM and prevent
    // debugging statements from being accidentally left in the codebase.
    // `console.error/warn` is still useful for error reporting from our users
    // (most of them know how to work with the browser dev console). But they
    // should be accessed via `@/common/tools/logging`.
    'no-console': ['error'],

    // Modify the prettier config to make it match the eslint rule from other
    // presets better.
    'prettier/prettier': [
      'error',
      {
        singleQuote: true,
      },
    ],

    // Ban the usage of `dangerouslySetInnerHTML`.
    //
    // Note that this rule does not catch the usage of `dangerouslySetInnerHTML`
    // in non-native components [1].
    // [1]: https://github.com/jsx-eslint/eslint-plugin-react/issues/3434
    'react/no-danger': ['error'],

    // See https://emotion.sh/docs/eslint-plugin-react.
    'react/no-unknown-property': ['error', { ignore: ['css'] }],

    // JSDoc related rules are deprecated [1].
    // Also with TypeScript, a lot of the JSDoc are unnecessary.
    // [1]: https://eslint.org/blog/2018/11/jsdoc-end-of-life/
    'require-jsdoc': 0,
    'valid-jsdoc': 0,
  },
  overrides: [
    {
      files: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
      plugins: ['jest'],
      extends: ['plugin:jest/recommended'],
    },
    {
      files: ['src/**/*.test.ts', 'src/**/*.test.tsx', '**/testing_tools/**'],
      rules: {
        // Allow assertion to make it easier to write test cases.
        // All incorrect assertion will be caught during test execution anyway.
        '@typescript-eslint/no-non-null-assertion': 0,

        // It's very common to use an empty mock implementation in tests.
        '@typescript-eslint/no-empty-function': 0,

        // Don't need to restrict imports in test files.
        'no-restricted-imports': 0,
      },
    },
  ],
};
