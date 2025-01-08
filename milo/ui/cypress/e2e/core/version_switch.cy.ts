// Copyright 2025 The LUCI Authors.
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

/// <reference types="cypress" />

/**
 * Some commands need to wait until the service worker finishes updating, which
 * can take a bit of time.
 */
const SW_RELATED_TIMEOUT_MS = 100000;

describe('can switch between new and old version', () => {
  beforeEach(() => {
    cy.clearAllCookies();
  });

  [
    {
      title: 'on index page',
      path: '/ui/',
    },
    {
      title: 'on always-fail page',
      path: '/ui/internal/always-fail',
    },
  ].forEach(({ title, path }) => {
    it(title, () => {
      cy.visit(path);

      // Switch to the old version.
      cy.get('[aria-label="Open menu"]').click();
      cy.contains('Switch to old UI')
        .parents('[role="menuitem"]')
        // Unsure why cypress thinks the element is covered by a `<p>` when it's
        // not.
        .click({ force: true });
      // Check whether we are on the old version.
      cy.get('[aria-label="Old UI Version Banner"]', {
        timeout: SW_RELATED_TIMEOUT_MS,
      }).should('be.visible');

      // Version selection is persisted through reloads.
      cy.reload({ timeout: SW_RELATED_TIMEOUT_MS });
      cy.get('[aria-label="Old UI Version Banner"]').should('be.visible');

      // Switch back to the new version.
      cy.get('[aria-label="Open menu"]').click();
      cy.contains('Switch to new UI')
        .parents('[role="menuitem"]')
        // Unsure why cypress thinks the element is covered by a `<p>` when it's
        // not.
        .click({ force: true });
      cy.get('[aria-label="Old UI Version Banner"]', {
        timeout: SW_RELATED_TIMEOUT_MS,
      }).should('not.exist');

      // URL is persisted.
      cy.location('pathname').should('equal', path);
    });
  });
});
