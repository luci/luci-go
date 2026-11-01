// Copyright 2026 The LUCI Authors.
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

import fconPlugin from './eslint-plugin-fcon-rules/index.js';

/** @type {import('eslint').Linter.Config[]} */
export default [
  {
    files: ['**/*.{js,mjs,cjs,ts,jsx,tsx}'],
    plugins: { 'fcon-rules': fconPlugin },
    rules: {
      'local-rules/canonical-go-links': 'error',
      'fcon-rules/no-window-location-hostname': 'error',
      'fcon-rules/require-create-fleet-feature-flag': 'error',
      'fcon-rules/no-legacy-imports': 'error',
    },
  },
  {
    files: ['features.ts', '**/*.test.{ts,tsx}'],
    rules: {
      'fcon-rules/require-create-fleet-feature-flag': 'off',
    },
  },
  {
    files: ['**/*.test.{ts,tsx}'],
    rules: {
      'jest/expect-expect': [
        'error',
        { assertFunctionNames: ['expect', 'verify*', 'then.*'] },
      ],
    },
  },
  {
    files: [
      'testing_tools/**',
      'components/fc_data_table/mrt_filter_menu_item.tsx',
    ],
    rules: {
      'fcon-rules/no-legacy-imports': 'off',
    },
  },
];
