// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.


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
    'plugin:jest/recommended',
    'plugin:import/recommended',
    'plugin:import/typescript',
    'plugin:jsx-a11y/recommended',
  ],
  'settings': {
    'react': {
      'version': 'detect',
    },
  },
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
    'jest',
    'jsx-a11y',
  ],
  'rules': {
    'quotes': ['error', 'single'],
    'semi': ['error', 'always'],
    'object-curly-spacing': ['error', 'always', { 'objectsInObjects': true }],
    'require-jsdoc': 0,
    'import/order': ['error'],
    'no-trailing-spaces': 'error',
    'no-console': ['error', { allow: ['error'] }],
    'eol-last': ['error', 'always'],
    'react/jsx-uses-react': 'off',
    'react/react-in-jsx-scope': 'off',
  },
};
