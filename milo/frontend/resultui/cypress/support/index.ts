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
import { addUnregisterServiceWorkersCommands } from './unregister_service_workers';

addMatchImageSnapshotCommand({
  failureThreshold: 0.01,
});
addStubRequestsCommand();
addStubPrpcServicesCommand();
addUnregisterServiceWorkersCommands();

beforeEach(() => {
  cy.stubPrpcServices();
});

afterEach(() => {
  // Cypress cancels all the pending network requests when a test case finished
  // running, which may cause errors. Those errors may in turn cause other tests
  // to fail. Add a small buffer after each test to avoid this.
  cy.wait(1000);
});
