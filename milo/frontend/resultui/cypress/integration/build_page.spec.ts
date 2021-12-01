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

import { RouteHandlerController } from 'cypress/types/net-stubbing';

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

  it('should refresh access token periodically', () => {
    let queryDelay = 0;
    let lastQueryTime = Date.now();

    function recordQueryTime(handler: RouteHandlerController): RouteHandlerController {
      return (req) => {
        const thisQueryTime = Date.now();
        queryDelay = thisQueryTime - lastQueryTime;
        lastQueryTime = thisQueryTime;
        return handler(req);
      };
    }

    cy.intercept(
      '/auth-state',
      recordQueryTime((req) =>
        req.reply({
          identity: 'user:test@test.com',
          email: 'test@test.com',
          Picture: '',
          accessToken: '',
          accessTokenExpiry: (Date.now() + 30000) / 1000,
        })
      )
    ).as('first-auth-state-request');
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252');
    // The first query should happen right after page load.
    cy.wait('@first-auth-state-request', { timeout: 4000 });
    cy.get('milo-signin').contains('test@test.com');

    // Should refresh token periodically.
    cy.intercept(
      '/auth-state',
      recordQueryTime((req) =>
        req.reply({
          identity: 'user:test@test.com',
          email: 'test@test.com',
          Picture: '',
          accessToken: '',
          accessTokenExpiry: (Date.now() + 20000) / 1000,
        })
      )
    ).as('second-auth-state-request');
    // The second query should happen just slightly before the first token
    // expires.
    cy.wait('@second-auth-state-request', { timeout: 30000 }).then(() => {
      expect(queryDelay).to.be.greaterThan(20000);
    });
    cy.get('milo-signin').contains('test@test.com');

    // Signed out.
    cy.intercept(
      '/auth-state',
      recordQueryTime((req) => req.reply({ identity: 'anonymous:anonymous' }))
    ).as('third-auth-state-request');
    // The third query should happen just slightly before the second token
    // expires.
    cy.wait('@third-auth-state-request', { timeout: 20000 }).then(() => {
      expect(queryDelay).to.be.greaterThan(10000);
    });
    cy.get('milo-signin').contains('Login');
  });

  it('should not refresh auth state too frequently', () => {
    let queryDelay = 0;
    let lastQueryTime = Date.now();

    function recordQueryTime(handler: RouteHandlerController): RouteHandlerController {
      return (req) => {
        const thisQueryTime = Date.now();
        queryDelay = thisQueryTime - lastQueryTime;
        lastQueryTime = thisQueryTime;
        return handler(req);
      };
    }

    const getAuthTokenExpiresImmediately: RouteHandlerController = (req) => {
      req.reply({
        identity: 'user:test@test.com',
        email: 'test@test.com',
        Picture: '',
        accessToken: '',
        accessTokenExpiry: Date.now() / 1000,
      });
    };

    cy.intercept('/auth-state', recordQueryTime(getAuthTokenExpiresImmediately)).as('first-auth-state-request');
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252');
    // The first query should happen right after page load.
    cy.wait('@first-auth-state-request').then(() => {
      expect(queryDelay).to.be.lessThan(5000);
    });

    // There should be some gap between each query even if the token expires
    // immediately.
    cy.intercept('/auth-state', recordQueryTime(getAuthTokenExpiresImmediately)).as('second-auth-state-request');
    cy.wait('@second-auth-state-request', { timeout: 15000 }).then(() => {
      expect(queryDelay).to.be.greaterThan(5000);
    });

    // Test again.
    cy.intercept('/auth-state', recordQueryTime(getAuthTokenExpiresImmediately)).as('third-auth-state-request');
    cy.wait('@third-auth-state-request', { timeout: 15000 }).then(() => {
      expect(queryDelay).to.be.greaterThan(5000);
    });
  });

  it('should refresh cached auth state immediately', () => {
    // Populate auth state cache.
    cy.intercept('/auth-state', (req) => {
      req.reply({
        identity: 'user:test@test.com',
        email: 'test@test.com',
        Picture: '',
        accessToken: '',
        accessTokenExpiry: Date.now() + 100000000 / 1000,
      });
    });
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252');
    cy.get('milo-signin').contains('test@test.com');

    // Auth state should still be queried after a reload even when the cached
    // state has not expired.
    cy.intercept('/auth-state', (req) => req.reply({ identity: 'anonymous:anonymous' })).as('request-after-refresh');
    cy.reload();
    cy.wait('@request-after-refresh', { timeout: 4000 });
    cy.get('milo-signin').contains('Login');
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
