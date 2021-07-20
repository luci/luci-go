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

describe('Test Results Tab', () => {
  it('config table modal should not be overlapped by other elements', () => {
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252/test-results');
    cy.get('milo-tvt-config-widget').click();
    cy.wait(1000);
    cy.scrollTo('topLeft');
    cy.matchImageSnapshot('config-table-modal', { capture: 'viewport' });
  });

  it('should show a warning banner when the build or one of the steps infra failed', () => {
    cy.visit('p/chromium/builders/ci/win-rel-swarming/11864/test-results');
    cy.get('#test-results-tab-warning').contains('Test results displayed here are likely incomplete');
  });

  it("should not show a warning banner when there's no infra failure", () => {
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252/test-results');
    cy.get('#test-results-tab-warning').should('not.exist');
  });
});
