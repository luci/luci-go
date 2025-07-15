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

import type { Config } from 'jest';

const config: Config = {
  // Use `js-with-babel` instead of `js-with-ts` because
  // 1. `js-with-babel` is significantly faster than `js-with-ts`. This is
  // likely because babel transpiles each file individually while tsc compiles
  // the entire project as a whole.
  // 2. The production build is also transpiled by babel. Using the same
  // transpiler helps ensuring the code under test behaves similar to the code
  // in production. Ideally, we should build testing code with the same
  // toolchain as we build production code (i.e. Vite). However, there's no
  // plugin supporting that yet.
  preset: 'ts-jest/presets/js-with-babel',
  testEnvironment: 'jsdom',
  // Match all JS/TS files under `**/__tests__/**` and all JS/TS files with
  // extension `.test` before the JS/TS extension. Unlike the default patterns,
  // `test.ts` is not treated as a test file since `test` in `test.ts` is not an
  // extension.
  testMatch: ['**/__tests__/**/*.[jt]s?(x)', '**/*.test.[jt]s?(x)'],
  // Some modules use `es6` modules, which is not compatible with jest, so we
  // need to transform them.
  transformIgnorePatterns: ['/node_modules/(?!lodash-es|lit|markdown-it)/'],
  globalSetup: './src/testing_tools/global_setup.ts',
  setupFilesAfterEnv: ['./src/testing_tools/setup_after_env.ts'],

  transform: {
    // The default transform from `ts-jest` preset doesn't handle `.mjs` files.
    '\\.mjs$': 'babel-jest',
  },
  // Reduce the time of test runs by caching unchanged test results.
  cacheDirectory: '.cache/jest',

  moduleNameMapper: {
    '\\.(css|less)$': 'identity-obj-proxy',
    '\\.(svg|md|png)($|\\?)': '<rootDir>/src/testing_tools/mocks/file_mock.ts',
    // Support custom path mapping declared in tsconfig.json.
    '^@/(.*)': '<rootDir>/src/$1',
    '^@root/(.*)': '<rootDir>/$1',
  },

  reporters: [
    'default',
    [
      'jest-slow-test-reporter',
      { numTests: 10, warnOnSlowerThan: 300, color: true },
    ],
    // Enable ResultDB integration. This is experimental. If you want to use
    // this, please contact chops-luci-test@google.com.
    [
      '<rootDir>/generated/resultdb_reporter.cjs',
      {
        repo: 'chromium.googlesource.com/infra/luci/luci-go',
        directory: 'milo/ui',
        delimiter: ' > ',
      },
    ],
  ],
};

export default config;
