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

// Tell TypeScript that the global Window object may have a 'gapi' property.
// Tell TypeScript that the global Window object may have a 'gapi' property.
declare global {
  interface Window {
    gapi: unknown;
  }
}

beforeEach(() => {
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
