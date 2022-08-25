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

describe('Clusters Page', () => {
  beforeEach(() => {
    cy.visit('/').contains('LOGIN').click();
    cy.get('body').contains('Logout');
    cy.visit('/p/chromium/clusters');
  });
  it('loads rules table', () => {
    // Navigate to the bug cluster page
    cy.contains('Rules').click();
    // check for the header text in the bug cluster table.
    cy.contains('Rule Definition');
  });
  it('loads cluster table', () => {
    // check for an entry in the cluster table.
    cy.get('[data-testid=clusters_table_body]').contains('test = ');
  });
  it('loads a cluster page', () => {
    cy.get('[data-testid=clusters_table_title] > a').first().click();
    cy.get('body').contains('Recent Failures');
    // Check that the analysis section is showing at least one group.
    cy.get('[data-testid=failures_table_group_cell]');
  });
});
