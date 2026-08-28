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

describe('Browser Devices Page', () => {
  beforeEach(() => {
    cy.clearLocalStorage();
    cy.window().then((win) => {
      win.indexedDB.deleteDatabase('keyval-store');
    });

    mockCypressAuth();

    const dimensionsData = {
      baseDimensions: { machine: { values: ['machine1'] } },
      swarmingLabels: {
        os: { values: ['Linux', 'Windows'] },
        pool: { values: ['chrome.tests'] },
      },
      ufsLabels: {
        model: { values: ['model1'] },
      },
    };

    mockPrpcEndpoint(
      'GetBrowserDeviceDimensions',
      dimensionsData,
      'getDimensions',
    );
    mockPrpcEndpoint(
      'ListBrowserDevices',
      {
        devices: [{ id: '1', ufsLabels: { os: { values: ['Linux'] } } }],
        totalSize: 1,
      },
      'listDevices',
    );
    mockPrpcEndpoint('CountBrowserDevices', undefined, 'countDevices');
  });

  it('should load with filters and render chips', () => {
    const targetUrl = '/ui/fleet/p/chromium/devices';

    cy.visit(targetUrl);

    cy.wait(['@listDevices', '@countDevices']);

    cy.get('input[placeholder*="Add a filter"]').click();
    cy.get('[role="menuitem"]').should('have.length.gt', 0);

    cy.get('[role="menuitem"]').contains('sw.os').click();
    cy.get('[role="menuitem"]').contains('Linux').click();
    cy.contains('button', 'Apply').click();

    cy.contains('[role="button"]', 'os').should('be.visible');
    cy.contains('[role="button"]', 'Linux').should('be.visible');

    cy.get('table').should('be.visible');
    cy.get('table').find('td').contains('1').should('be.visible');
  });
});
