// Copyright 2021 The LUCI Authors.
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

const RAW_ARTIFACT_PAGE_URL =
  '/ui/artifact/raw/invocations/task-chromium-swarm-dev.appspot.com-53d2baa7033f4711/tests/' +
  'ninja%3A%2F%2Fchrome%2Ftest%3Ainteractive_ui_tests%2FMediaDialogViewBrowserTest.PlayingSessionAlwaysDisplayFirst' +
  '/results/f66c3e76-00731/artifacts/snippet';

describe('Raw Artifact Page', () => {
  beforeEach(() => {
    cy.stubRequests({ hostname: 'staging.results.usercontent.cr.dev' }, 'artifact', { matchHeaders: [] });
  });

  it('should render raw artifact when the service worker is not registered', () => {
    cy.unregisterServiceWorkers('/ui/');

    cy.visit(RAW_ARTIFACT_PAGE_URL);
    cy.get('pre').contains('MediaDialogViewBrowserTest.PlayingSessionAlwaysDisplayFirst');

    const resultDbArtifactUrl =
      '/invocations/task-chromium-swarm-dev.appspot.com-53d2baa7033f4711/tests/' +
      'ninja%3A%2F%2Fchrome%2Ftest%3Ainteractive_ui_tests%2FMediaDialogViewBrowserTest' +
      '.PlayingSessionAlwaysDisplayFirst/results/f66c3e76-00731/artifacts/snippet';

    cy.location('pathname').should('equal', resultDbArtifactUrl);
    cy.location('origin').should('equal', 'https://staging.results.usercontent.cr.dev');
  });

  it('should render raw artifact without URL redirection when the service worker is registered', () => {
    // Ensures the service worker is loaded.
    cy.intercept('/sw-loaded', (req) => req.reply('')).as('sw-loaded');
    cy.visit('/ui/');
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    cy.window().then((win) => (win as any).SW_PROMISE.then(() => window.fetch('/sw-loaded')));
    cy.wait('@sw-loaded');

    cy.visit(RAW_ARTIFACT_PAGE_URL);

    cy.get('pre').contains('MediaDialogViewBrowserTest.PlayingSessionAlwaysDisplayFirst');
    cy.location('pathname').should('equal', RAW_ARTIFACT_PAGE_URL);
    cy.location('origin').should('equal', Cypress.config().baseUrl);
  });
});
