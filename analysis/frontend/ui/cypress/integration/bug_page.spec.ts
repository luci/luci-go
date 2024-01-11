// Copyright 2022 The LUCI Authors.
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
import { setupTestRule } from './test_data';

describe('Bug Page', () => {
  beforeEach(() => {
    // Login.
    cy.visit('/').contains('Log in').click();
    cy.contains('LOGIN').click();

    setupTestRule();
  });

  it('redirects if single matching rule found', () => {
    cy.visit('/b/chromium/920867');
    cy.get('[data-testid=bug]').contains('crbug.com/920867');
  });

  it('no matching rule exists', () => {
    cy.visit('/b/chromium/404');
    cy.get('bug-page').contains('No rule found matching the specified bug (monorail:chromium/404).');
  });

  it('multiple matching rules found', () => {
    cy.intercept('POST', '/prpc/luci.analysis.v1.Rules/LookupBug', (req) => {
      const requestBody = req.body;
      assert.deepEqual(requestBody, { system: 'monorail', id: 'chromium/1234' });

      const response = {
        // This is a real rule that exists in the dev database, the
        // same used for rule section UI tests.
        rules: [
          'projects/chromium/rules/ea5305bc5069b449ee43ee64d26d667f',
          'projects/chromiumos/rules/1234567890abcedf1234567890abcdef',
        ],
      };
      // Construct pRPC response.
      const body = ')]}\'\n' + JSON.stringify(response);
      req.reply(body, {
        'X-Prpc-Grpc-Code': '0',
      });
    }).as('lookupBug');

    cy.visit('/b/chromium/1234');
    cy.wait('@lookupBug');

    cy.get('body').contains('chromiumos');
    cy.get('body').contains('chromium').click();

    cy.get('[data-testid=rule-definition]').contains('test = "cypress test 1"');
  });
});
