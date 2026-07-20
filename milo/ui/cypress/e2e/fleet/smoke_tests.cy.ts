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

import { mockPrpc } from './common/utils';

describe('Fleet Console Smoke Tests', () => {
  beforeEach(() => {
    cy.clearLocalStorage();

    // Mock auth state to be a Googler so that protected routes are accessible.
    cy.intercept('GET', '**/auth/openid/state', {
      body: {
        identity: 'user:user@google.com',
        email: 'user@google.com',
      },
    });

    // Common mocks for device dimensions and list to prevent crashes on pages that use them.
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/GetDeviceDimensions',
      { baseDimensions: {}, labels: {} },
      'getDimensions',
    );
  });

  it('should load Home Page', () => {
    cy.visit('/ui/fleet');
    cy.contains('Fleet Console').should('be.visible');
    cy.contains('ChromeOS').should('be.visible');
    cy.contains('Android').should('be.visible');
  });

  it('should load Android Repairs Page', () => {
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/GetRepairMetricsDimensions',
      { dimensions: {} },
      'getRepairDimensions',
    );
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/ListRepairMetrics',
      {
        repairMetrics: [
          { labName: 'lab1', totalDevices: 10, devicesOffline: 2 },
        ],
      },
      'listRepairMetrics',
    );
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/CountRepairMetrics',
      { total: 1 },
      'countRepairMetrics',
    );

    cy.visit('/ui/fleet/p/android/repairs');
    cy.wait([
      '@getRepairDimensions',
      '@listRepairMetrics',
      '@countRepairMetrics',
    ]);
    cy.contains('Repair metrics').should('be.visible');
    cy.get('table').should('be.visible');
    cy.contains('lab1').should('be.visible');
  });

  it('should load Product Catalogue Page', () => {
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/ListProductCatalogEntries',
      { entries: [{ productCatalogId: 'prod1', productName: 'Product 1' }] },
      'listCatalog',
    );
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/GetProductCatalogFilterValues',
      { productCatalogId: [] },
      'getCatalogFilters',
    );

    cy.visit('/ui/fleet/catalog');
    cy.wait(['@listCatalog', '@getCatalogFilters']);
    cy.get('input[placeholder*="Add a filter"]').should('be.visible');
    cy.contains('Product 1').should('be.visible');
  });

  it('should load Resource Request Insights Page', () => {
    cy.visit('/ui/fleet/requests');
    cy.get('input[placeholder*="Add a filter"]').should('be.visible');
  });

  it('should load Metrics Page', () => {
    cy.visit('/ui/fleet/metrics');
    cy.get('iframe[title="North Star Metrics"]').should('be.visible');
  });

  it('should load Resource Planner Insights Page', () => {
    cy.visit('/ui/fleet/planners');
    cy.get('iframe[title="Resource Planner Insights"]').should('be.visible');
  });

  it('should load Admin Tasks Page', () => {
    mockPrpc('**/prpc/swarming.v2.Tasks/ListTasks', { items: [] }, 'listTasks');

    cy.visit('/ui/fleet/p/chromeos/admin-tasks');
    cy.contains('ChromeOS Admin Tasks').should('be.visible');
    cy.contains('Active Tasks').should('be.visible');
    cy.contains('Task History').should('be.visible');
  });
});
