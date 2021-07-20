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

import { addMatchImageSnapshotCommand } from 'cypress-image-snapshot/command';

import { addStubPrpcServicesCommand } from './stub_prpc_services';
import { addStubRequestsCommand } from './stub_requests';

addMatchImageSnapshotCommand({
  failureThreshold: 0.01,
});
addStubRequestsCommand();
addStubPrpcServicesCommand();

beforeEach(() => {
  cy.stubPrpcServices();

  // Cypress cancels all the pending network requests when a test case
  // finished running. Ignore errors due to network requests being canceled.
  cy.on('uncaught:exception', (e) => !e.message.includes('> NetworkError when attempting to fetch resource.'));
});

afterEach(() => {
  // Sometimes errors could happen in afterEach hooks. Ignore those errors.
  cy.on('uncaught:exception', () => {});
});
