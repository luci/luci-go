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

describe('New Rule Page', () => {
  beforeEach(() => {
    // Login.
    cy.visit('/').contains('Log in').click();
    cy.contains('LOGIN').click();
  });
  it('create rule from scratch', () => {
    cy.visit('/p/chromium/rules/new');

    cy.get('new-rule-page').get('[data-cy=bug-system-dropdown]').contains('crbug.com');
    cy.get('new-rule-page').get('[data-cy=bug-number-textbox]').get('[type=text]').type('{selectall}101');
    cy.get('new-rule-page').get('[data-cy=rule-definition-textbox]').get('textarea').type('{selectall}test = "create test 1"');

    cy.intercept('POST', '/prpc/luci.analysis.v1.Rules/Create', (req) => {
      const requestBody = req.body;
      assert.strictEqual(requestBody.rule.ruleDefinition, 'test = "create test 1"');
      assert.deepEqual(requestBody.rule.bug, { system: 'monorail', id: 'chromium/101' });
      assert.deepEqual(requestBody.rule.sourceCluster, { algorithm: '', id: '' });

      const response = {
        project: 'chromium',
        // This is a real rule that exists in the dev database, the
        // same used for rule section UI tests.
        ruleId: 'ea5305bc5069b449ee43ee64d26d667f',
      };
      // Construct pRPC response.
      const body = ')]}\'\n' + JSON.stringify(response);
      req.reply(body, {
        'X-Prpc-Grpc-Code': '0',
      });
    }).as('createRule');

    cy.get('new-rule-page').get('[data-cy=create-button]').click();
    cy.wait('@createRule');

    // Verify the rule page loaded.
    cy.get('body').contains('Associated Bug');
  });
  it('create rule from cluster', () => {
    // Use an invalid rule to ensure it does not get created in dev by
    // accident.
    const rule = 'test = CREATE_TEST_2';
    cy.visit(`/p/chromium/rules/new?rule=${encodeURIComponent(rule)}&sourceAlg=reason-v1&sourceId=1234567890abcedf1234567890abcedf`);

    cy.get('new-rule-page').get('[data-cy=bug-system-dropdown]').contains('crbug.com');
    cy.get('new-rule-page').get('[data-cy=bug-number-textbox]').get('[type=text]').type('{selectall}101');

    cy.intercept('POST', '/prpc/luci.analysis.v1.Rules/Create', (req) => {
      const requestBody = req.body;
      assert.strictEqual(requestBody.rule.ruleDefinition, 'test = CREATE_TEST_2');
      assert.deepEqual(requestBody.rule.bug, { system: 'monorail', id: 'chromium/101' });
      assert.deepEqual(requestBody.rule.sourceCluster, { algorithm: 'reason-v1', id: '1234567890abcedf1234567890abcedf' });

      const response = {
        project: 'chromium',
        // This is a real rule that exists in the dev database, the
        // same used for rule section UI tests.
        ruleId: 'ea5305bc5069b449ee43ee64d26d667f',
      };
      // Construct pRPC response.
      const body = ')]}\'\n' + JSON.stringify(response);
      req.reply(body, {
        'X-Prpc-Grpc-Code': '0',
      });
    }).as('createRule');

    cy.get('new-rule-page').get('[data-cy=create-button]').click();
    cy.wait('@createRule');

    // Verify the rule page loaded.
    cy.get('body').contains('Associated Bug');
  });
  it('displays validation errors', () => {
    cy.visit('/p/chromium/rules/new');
    cy.get('new-rule-page').get('[data-cy=bug-system-dropdown]').contains('crbug.com');
    cy.get('new-rule-page').get('[data-cy=bug-number-textbox]').get('[type=text]').type('{selectall}101');
    cy.get('new-rule-page').get('[data-cy=rule-definition-textbox]').get('textarea').type('{selectall}test = INVALID');

    cy.get('new-rule-page').get('[data-cy=create-button]').click();

    cy.get('body').contains('Validation error: rule definition is not valid: undeclared identifier "invalid".');
  });
});
