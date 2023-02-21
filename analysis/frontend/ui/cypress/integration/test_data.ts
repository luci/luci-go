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

export function setupTestRule() {
  cy.request({
    url: '/api/authState',
    headers: {
      'Sec-Fetch-Site': 'same-origin',
    },
  }).then((response) => {
    assert.strictEqual(response.status, 200);
    const body = response.body;
    const accessToken = body.accessToken;
    assert.isString(accessToken);
    assert.notEqual(accessToken, '');

    // Set initial rule state.
    cy.request({
      method: 'POST',
      url: '/prpc/luci.analysis.v1.Rules/Update',
      body: {
        rule: {
          name: 'projects/chromium/rules/ea5305bc5069b449ee43ee64d26d667f',
          ruleDefinition: 'test = "cypress test 1"',
          bug: {
            system: 'monorail',
            id: 'chromium/920867',
          },
          isActive: true,
          isManagingBug: true,
        },
        updateMask: 'ruleDefinition,bug,isActive,isManagingBug',
      },
      headers: {
        Authorization: 'Bearer ' + accessToken,
      },
    });
  });
}
