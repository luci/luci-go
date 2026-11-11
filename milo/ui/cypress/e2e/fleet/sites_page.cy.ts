// Copyright 2026 The LUCI Authors.
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

import { mockCypressAuth, mockPrpcEndpoint } from './common/utils';

describe('Physical Sites Directory Page', () => {
  beforeEach(() => {
    cy.clearLocalStorage();
    cy.on('window:before:load', (win) => {
      win.localStorage.setItem('fleet_console_shortcuts_tip_dismissed', 'true');
    });

    mockCypressAuth();
    mockPrpcEndpoint(
      'CheckPermission',
      { hasPermission: true },
      'checkPermission',
    );
    mockPrpcEndpoint(
      'ListPhysicalSites',
      {
        physicalSites: [
          { id: 'MTV', name: 'Mountain View MTV' },
          { id: 'EM25', name: 'Palo Alto EM25' },
        ],
      },
      'listPhysicalSites',
    );
  });

  it('should render physical site cards and filter by search query', () => {
    cy.visit('/ui/fleet/sites?ff=physical-sites:on');
    cy.wait('@listPhysicalSites');

    cy.contains('h1', 'Physical sites').should('be.visible');
    cy.get('[data-testid="site-card-MTV"]').should(
      'contain.text',
      'Mountain View MTV',
    );
    cy.get('[data-testid="site-card-EM25"]').should(
      'contain.text',
      'Palo Alto EM25',
    );

    cy.get('input[aria-label="Filter physical sites"]').type('EM25');
    cy.get('[data-testid="site-card-EM25"]').should('be.visible');
    cy.get('[data-testid="site-card-MTV"]').should('not.exist');
  });

  it('should support creating, editing, and deleting a physical site', () => {
    mockPrpcEndpoint(
      'CreatePhysicalSite',
      { physicalSite: { id: 'IAD65', name: 'Ashburn IAD65' } },
      'createPhysicalSite',
    );
    mockPrpcEndpoint(
      'UpdatePhysicalSite',
      { physicalSite: { id: 'EM25', name: 'Palo Alto EM25 Lab' } },
      'updatePhysicalSite',
    );
    mockPrpcEndpoint('DeletePhysicalSite', {}, 'deletePhysicalSite');

    cy.visit('/ui/fleet/sites?ff=physical-sites:on');
    cy.wait('@listPhysicalSites');
    cy.wait('@checkPermission');

    cy.contains('button', 'Add site').click();
    cy.get('[role="dialog"]')
      .find('input[placeholder="e.g. US-PAO-EM25"]')
      .type('IAD65');
    cy.get('[role="dialog"]')
      .find('input[placeholder="e.g. Palo Alto EM25"]')
      .type('Ashburn IAD65');
    cy.get('[role="dialog"]').contains('button', 'Add site').click();
    cy.wait('@createPhysicalSite');

    cy.get('button[aria-label="Edit Palo Alto EM25"]').click();
    cy.get('[role="dialog"]')
      .find('input[placeholder="e.g. Palo Alto EM25"]')
      .clear()
      .type('Palo Alto EM25 Lab');
    cy.get('[role="dialog"]').contains('button', 'Save changes').click();
    cy.wait('@updatePhysicalSite');

    cy.get('button[aria-label="More actions for Mountain View MTV"]').click();
    cy.get('[role="menuitem"][aria-label="Delete Mountain View MTV"]').click();
    cy.get('[role="dialog"]').contains('button', 'Delete site').click();
    cy.wait('@deletePhysicalSite');
  });

  it('should render Page not found when physical-sites feature flag is disabled', () => {
    cy.visit('/ui/fleet/sites?ff=physical-sites:off');
    cy.contains('Page not found').should('be.visible');
  });
});
