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

    // We need to import Lit component definition file separately as a
    // side-effect (e.g. `import './a_component'`) because type-only imports
    // (e.g. `import { AComponent } from './a_component'`) may get optimized
    // away by the bundler.
    'import/no-duplicates': 0,

    // Group internal dependencies together.
    'import/order': [
      'error',
      {
        pathGroups: [
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

    // TypeScript and LitElement template is hard to fit in 80 cols.
    // Note that the prettier will still try to reformat the code into 80 cols.
    // But the prettier does not apply a hard limit.
    'max-len': [
      'error',
      { code: 120, ignoreUrls: true, ignoreRegExpLiterals: true },
    ],

    // console.error/warn is still useful for error reporting from our users
    // (most of them know how to work with the browser dev console).
    // Ban the usage of console.log to prevent temporary debug code from being
    // merged.
    'no-console': ['error', { allow: ['error', 'warn'] }],

    // Modify the prettier config to make it match the eslint rule from other
    // presets better.
    'prettier/prettier': [
      'error',
      {
        singleQuote: true,
      },
    ],

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
      files: ['cypress/**'],
      plugins: ['cypress'],
      extends: ['plugin:cypress/recommended'],
    },
    {
      files: ['src/**/*.test.ts', 'src/**/*.test.tsx', 'cypress/**'],
      rules: {
        // Allow assertion to make it easier to write test cases.
        // All incorrect assertion will be caught during test execution anyway.
        '@typescript-eslint/no-non-null-assertion': 0,

        // It's very common to use an empty mock implementation in tests.
        '@typescript-eslint/no-empty-function': 0,
      },
    },
  ],
};