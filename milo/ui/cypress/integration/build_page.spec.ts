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

import { STUB_REQUEST_OPTIONS } from '../support/stub_prpc_services';

describe('Build Page', () => {
  it('should navigate to the default tab', () => {
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252');
    cy.location('pathname').should('equal', '/ui/p/chromium/builders/ci/linux-rel-swarming/15252/overview');
  });

  it('should initiate the signin flow if the page is 404 and the user is not logged in', () => {
    cy.visit('/p/not-bound-project/builders/not-bound-bucket/not-found-builder/12479');
    cy.on('uncaught:exception', () => false);
    cy.location('pathname').should('equal', '/ui/login');
  });

  it('should compute invocation ID from buildNum in URL', () => {
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252');
    cy.get('milo-build-page')
      .invoke('prop', 'buildState')
      .its('invocationId')
      .should('eq', 'build-70535a5a746775ce83281f4e4e318b2b7b239d1e7eb7c8f790bf570a14cf61fe-15252');
  });

  it('should compute invocation ID from build ID in URL', () => {
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/b8845866669318458401');
    cy.get('milo-build-page')
      .invoke('prop', 'buildState')
      .its('invocationId')
      .should('eq', 'build-8845866669318458401');
  });

  it('should fallback to invocation ID from buildbucket when invocation is not found', () => {
    // modified-resultdb is manually modified to respond 404 to queries with
    // computed invocation IDs.
    cy.stubRequests(
      { url: 'https://staging.results.api.cr.dev/prpc/**', method: 'POST' },
      'modified-resultdb',
      STUB_REQUEST_OPTIONS
    );
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252');
    cy.on('uncaught:exception', () => false);
    cy.get('milo-build-page')
      .invoke('prop', 'buildState')
      .its('invocationId')
      .should('eq', 'build-8845866669318458401');
  });

  it('should redirect to a long link when visited via a short link', () => {
    cy.visit('/b/8845866669318458401');
    cy.location('pathname').should('equal', '/ui/p/chromium/builders/ci/linux-rel-swarming/15252/overview');
  });

  it('should not break browser back button after internal redirection', () => {
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252');
    cy.location('pathname').should('equal', '/ui/p/chromium/builders/ci/linux-rel-swarming/15252/overview');
    cy.get('#summary-html'); // Ensure the overview tab is loaded.

    // Navigate to a different page with a short build page URL.
    // Add + "/overview" to avoid 301 redirect.
    cy.visit('/b/8845863326460499505/overview');
    cy.location('pathname').should('equal', '/ui/p/chromium/builders/ci/linux-rel-swarming/15253/overview');
    cy.get('#summary-html'); // Ensure the overview tab is loaded.

    // Go to a different tab.
    cy.get('milo-tab-bar').contains('Test Results').click();
    cy.location('pathname').should('equal', '/ui/p/chromium/builders/ci/linux-rel-swarming/15253/test-results');
    cy.get('milo-test-variant-entry'); // Ensure the test results tab is loaded.

    // Go back to the overview tab of this build.
    cy.go('back');
    cy.location('pathname').should('equal', '/ui/p/chromium/builders/ci/linux-rel-swarming/15253/overview');
    cy.get('#summary-html'); // Ensure the overview tab is loaded.

    // Go back to the overview tab of the previous build.
    cy.go('back');
    cy.location('pathname').should('equal', '/ui/p/chromium/builders/ci/linux-rel-swarming/15252/overview');
    cy.get('#summary-html'); // Ensure the overview tab is loaded.
  });

  it('should display test name properly', () => {
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252/overview');
    cy.get('milo-test-variant-entry').then((ele) => {
      const rect = ele[0].getBoundingClientRect();
      cy.matchImageSnapshot('test-variant-entry', {
        capture: 'viewport',
        clip: { x: rect.left, y: rect.top, width: rect.width, height: rect.height },
      });
    });
  });
});
