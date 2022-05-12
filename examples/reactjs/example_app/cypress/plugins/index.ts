// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// eslint-disable-next-line spaced-comment
/// <reference types="cypress" />

/* eslint-disable import/no-anonymous-default-export */
// eslint-disable-next-line valid-jsdoc
/**
* @type {Cypress.PluginConfig}
*/
export default (
    _: Cypress.PluginEvents,
    config: Cypress.PluginConfigOptions,
) => {
  return Object.assign({}, config, {
    fixturesFolder: 'cypress/fixtures',
    integrationFolder: 'cypress/integration',
    screenshotsFolder: 'cypress/screenshots',
    videosFolder: 'cypress/videos',
    supportFile: 'cypress/support/index.ts',
  });
};
