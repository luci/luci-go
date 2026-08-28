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

describe('Fleet Console Smoke Tests', () => {
  beforeEach(() => {
    cy.clearLocalStorage();

    mockCypressAuth();

    mockPrpcEndpoint('GetDeviceDimensions', undefined, 'getDimensions');
  });

  it('should load Home Page', () => {
    cy.visit('/ui/fleet');
    cy.contains('Fleet Console').should('be.visible');
    cy.contains('ChromeOS').should('be.visible');
    cy.contains('Android').should('be.visible');
  });

  it('should load Android Repairs Page', () => {
    mockPrpcEndpoint(
      'GetRepairMetricsDimensions',
      { dimensions: {} },
      'getRepairDimensions',
    );
    mockPrpcEndpoint('ListRepairMetrics', undefined, 'listRepairMetrics');
    mockPrpcEndpoint('CountRepairMetrics', undefined, 'countRepairMetrics');

    cy.visit('/ui/fleet/p/android/repairs');
    cy.wait([
      '@getRepairDimensions',
      '@listRepairMetrics',
      '@countRepairMetrics',
    ]);
    cy.contains('Repair metrics').should('be.visible');
    cy.get('table').should('be.visible');
  });

  it('should load Product Catalogue Page', () => {
    mockPrpcEndpoint(
      'ListProductCatalogEntries',
      { entries: [{ productCatalogId: 'prod1', productName: 'Product 1' }] },
      'listCatalog',
    );
    mockPrpcEndpoint(
      'GetProductCatalogFilterValues',
      { productCatalogId: [] },
      'getCatalogFilters',
    );

    mockPrpcEndpoint(
      'ListGceProductCatalogEntries',
      { entries: [] },
      'listGceCatalog',
    );

    cy.visit('/ui/fleet/catalog');
    cy.wait(['@listCatalog', '@getCatalogFilters', '@listGceCatalog']);
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
    mockPrpcEndpoint('ListTasks', { items: [] }, 'listTasks');

    cy.visit('/ui/fleet/p/chromeos/admin-tasks');
    cy.contains('ChromeOS Admin Tasks').should('be.visible');
    cy.contains('Active Tasks').should('be.visible');
    cy.contains('Task History').should('be.visible');
  });
});
