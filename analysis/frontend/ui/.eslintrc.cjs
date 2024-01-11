// Copyright 2022 The LUCI Authors.
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
  'env': {
    'browser': true,
    'es2021': true,
  },
  'extends': [
    'eslint:recommended',
    'plugin:react/recommended',
    'plugin:react-hooks/recommended',
    'prettier',
    'google',
    'plugin:@typescript-eslint/recommended',
    'plugin:import/recommended',
    'plugin:import/typescript',
    'plugin:jsx-a11y/recommended',
  ],
  'settings': {
    'react': {
      'version': 'detect',
    },
    'import/parsers': {
      '@typescript-eslint/parser': ['.ts', '.tsx'],
    },
    'import/resolver': {
      'typescript': {},
    },
  },
  'overrides': [
    {
      'files': ['src/**/*.test.tsx'],
      'plugins': ['jest'],
      'extends': ['plugin:jest/recommended'],
    },
    {
      'files': ['cypress/**'],
      'plugins': ['cypress'],
      'extends': ['plugin:cypress/recommended'],
    },
  ],
  'parser': '@typescript-eslint/parser',
  'parserOptions': {
    'ecmaFeatures': {
      'jsx': true,
    },
    'ecmaVersion': 'latest',
    'sourceType': 'module',
  },
  'plugins': [
    'react',
    '@typescript-eslint',
    'prettier',
    'jsx-a11y',
    'import',
  ],
  'rules': {
    // Code generated from protobuf may contain '_' in the identifier name,
    // (e.g. `BuilderMask_BuilderMaskType`) and therefore triggering the error.
    // `"ignoreImports": true` doesn't fix the issue because checks are still
    // applied where the imported symbol is used.
    //
    // Since this rule doesn't add a lot of value (it only checks whether there
    // are '_' in non-leading/trailing positions), disable it to reduce noise.
    //
    // Note that we should still generally use camelcase.
    'camelcase': 0,
    'quotes': ['error', 'single'],
    'semi': ['error', 'always'],
    'object-curly-spacing': ['error', 'always', { 'objectsInObjects': true }],
    'require-jsdoc': 0,
    'import/order': ['error', {
      'pathGroups': [
        {
          'pattern': '@/**',
          'group': 'external',
          'position': 'after',
        },
      ],
    }],
    'import/no-unresolved': 'error',
    'no-trailing-spaces': 'error',
    'no-console': ['error', { allow: ['error'] }],
    'eol-last': ['error', 'always'],
    'react/jsx-uses-react': 'off',
    'react/react-in-jsx-scope': 'off',
    'max-len': 'off',
  },
};
