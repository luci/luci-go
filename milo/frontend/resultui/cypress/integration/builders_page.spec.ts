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

describe('Builders Page', () => {
  it('should get project ID from URL', () => {
    cy.visit('/ui/p/chromium/builders');
    cy.get('milo-builders-page').shadow().get('#builders-group-id').contains('chromium');
  });

  it('should get group ID from URL', () => {
    // We don't actually have a builders group in -dev.
    // Modify the RPC responses to not return 404.
    cy.stubRequests({ url: 'https://localhost:8080/prpc/**', method: 'POST' }, 'modified-milo', STUB_REQUEST_OPTIONS);

    cy.visit('/ui/p/chromium/g/builders-group/builders');
    cy.get('milo-builders-page').shadow().get('#builders-group-id').contains('chromium');
    cy.get('milo-builders-page').shadow().get('#builders-group-id').contains('builders-group');
  });

  it('should render builder rows', () => {
    cy.visit('/ui/p/chromium/builders');
    cy.get('milo-builders-page-row:first-child').contains('chromium/ci/android-marshmallow-arm64-rel-swarming');
    cy.get('milo-builders-page-row').should('have.length', 9);
    cy.get('#loading-row').contains('Showing 9 builders.');
  });
});
