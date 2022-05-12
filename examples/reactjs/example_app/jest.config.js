// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/** @type {import('ts-jest/dist/types').InitialOptionsTsJest} */
// eslint-disable-next-line no-undef
module.exports = {
  /**
     * This resolved many issues that may occur while importing 3rd party tools.
     */
  preset: 'ts-jest/presets/js-with-ts',
  /**
     * The test env is JSDom as it enables us to use
     * `window` and other browser specific variables.
     */
  testEnvironment: 'jsdom',
  /**
     * files with `.spec.ts` are reserved for cypress tests.
     */
  testMatch: [
    '**/__tests__/**/*.[jt]s?(x)',
    '**/?(*.)+(test).[jt]s?(x)',
  ],
  /**
     * The reason we need to set this is because we are
     * importing node_modules which are using
     * `es6` modules and that is not compatible with jest,
     * so we need to transform them.
     */
  transformIgnorePatterns: [
    '/node_modules/' +
     '(?!(node-fetch|data-uri-to-buffer|fetch-blob|formdata-polyfill)/)',
  ],
  /**
   * This allows components to import styling files without
   * failing the jest build.
   */
  moduleNameMapper: {
    '\\.(css|less|scss)$': 'identity-obj-proxy',
  },
  /**
     * This files initializes the required environment for jest and react.
     */
  setupFiles: [
    './src/testing_tools/setUpEnv.ts',
  ],
};
