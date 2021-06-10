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

describe('Build Page', () => {
  it('should navigate to the default tab', () => {
    cy.stubPrpcServices();
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252');
    cy.url().should('include', '/overview');
  });

  it('should initiate the signin flow if the page is 404 and the user is not logged in', () => {
    cy.stubPrpcServices();
    cy.visit('/p/not-bound-project/builders/not-bound-bucket/not-found-builder/12479');
    cy.on('uncaught:exception', () => false);
    cy.location('pathname').should('include', '/ui/login');
  });
});
