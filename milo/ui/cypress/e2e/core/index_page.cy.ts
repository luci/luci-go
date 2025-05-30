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

describe('visit index page', () => {
  it('index page loads', () => {
    cy.visit('/');

    cy.contains('Welcome to LUCI');
  });

  // TODO (b/406076495) Fix test by dismissing the what's new modal
  // if it was on screen.
  it.skip('can navigate to project page', () => {
    cy.visit('/');
    cy.contains('chromium').click();

    cy.contains('Chromium Main Console');
  });
});
