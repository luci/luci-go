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

module.exports = {
  preset: 'ts-jest/presets/js-with-babel',
  testEnvironment: 'jsdom',
  testMatch: [
    '**/__tests__/**/*.[jt]s?(x)',
    // Use the old test filename pattern (e.g. *_test.ts) to reduce git diff
    // during migration.
    // TODO(weiweilin): rename all unit test files and use the standard pattern
    // (e.g. *.test.ts).
    '**/?(*_test).[jt]s?(x)',
  ],
  transform: {
    '^.+\\.jsx?$': 'babel-jest',
    '^.+\\.[tj]sx?$': [
      'ts-jest',
      {
        // Jest requires `moduleResolution: "node"`. Use a separate tsconfig
        // file.
        tsconfig: 'tsconfig.test.json',
      },
    ],
  },
  /**
   * Some modules uses `es6` modules, which is not compatible with jest, so we
   * need to transform them.
   */
  transformIgnorePatterns: ['/node_modules/?!(lodash-es|lit)'],
  setupFiles: ['./src/testing_tools/setup_env.ts'],
  moduleNameMapper: {
    '\\.(css|less|svg)$': 'identity-obj-proxy',
  },
};
