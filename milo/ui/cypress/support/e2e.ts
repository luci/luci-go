// Copyright 2024 The LUCI Authors.
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

// Tell TypeScript that the global Window and Cypress namespaces have these custom properties/commands.
declare global {
  interface Window {
    gapi: unknown;
  }

  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace Cypress {
    interface Chainable {
      /**
       * Clears all IndexedDB databases in the current window context.
       */
      clearIndexedDB(): Chainable<void>;
      /**
       * Clears all client-side state (IndexedDB, LocalStorage, Cookies)
       * to isolate tests and prevent state leakage.
       */
      clearAllClientState(): Chainable<void>;
    }
  }
}

// Register custom command to clear IndexedDB
Cypress.Commands.add('clearIndexedDB', () => {
  return cy.window().then((win) => {
    if (!win.indexedDB || !win.indexedDB.databases) {
      return;
    }
    return win.indexedDB.databases().then((dbs) => {
      return Promise.all(
        dbs.map((db) => {
          return new Promise<void>((resolve, reject) => {
            if (!db.name) {
              resolve();
              return;
            }
            const req = win.indexedDB.deleteDatabase(db.name);
            req.onsuccess = () => resolve();
            req.onerror = () => reject(req.error);
            req.onblocked = () => {
              // eslint-disable-next-line no-console
              console.warn(
                `IndexedDB deletion blocked for database: ${db.name}`,
              );
              resolve(); // Resolve to avoid hanging the test suite
            };
          });
        }),
      ).then(() => {});
    });
  });
});

// Register custom command to clear all client-side state
Cypress.Commands.add('clearAllClientState', () => {
  cy.clearLocalStorage();
  cy.clearCookies();
  return cy.clearIndexedDB();
});

beforeEach(() => {
  // Automatically clear client state before every test across all specs to prevent state leakage
  cy.clearAllClientState();

  // Intercept the Google API script and return an empty response
  // to prevent it from loading and executing in tests.
  cy.intercept('https://apis.google.com/js/api.js', { body: '' });

  // Create a mock gapi object on the window before each test.
  cy.on('window:before:load', (win) => {
    win.gapi = {
      load: () => {},
      client: {
        init: () => {},
      },
    };
  });
});

// Add this empty export to treat this file as a module.
export {};
