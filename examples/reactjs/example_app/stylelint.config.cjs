// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
/* eslint-env node */

module.exports = {
  extends: [
    'stylelint-config-recommended',
    'stylelint-config-standard',
    'stylelint-config-recess-order',
    'stylelint-config-css-modules',
  ],
  plugins: ['stylelint-scss'],
  ignoreFiles: ['./coverage/**/*.css', './dist/**/*.css'],
  rules: {
    // --------
    // SCSS rules
    // --------
    'scss/dollar-variable-colon-space-before': 'never',
    'scss/dollar-variable-colon-space-after': 'always',
    'scss/dollar-variable-no-missing-interpolation': true,
    'scss/dollar-variable-pattern': /^[a-z-]+$/,
    'scss/double-slash-comment-whitespace-inside': 'always',
    'scss/operator-no-newline-before': true,
    'scss/operator-no-unspaced': true,
    'scss/selector-no-redundant-nesting-selector': true,
    // Allow SCSS and CSS module keywords beginning with `@`
    'scss/at-rule-no-unknown': null,
  },
};
