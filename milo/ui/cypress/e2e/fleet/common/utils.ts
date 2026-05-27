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

/**
 * Mocks a pRPC request with the correct XSSI prefix and headers.
 * @param url The URL pattern to intercept.
 * @param body The JSON response body object.
 * @param alias The Cypress alias to assign to the route.
 */
export function mockPrpc(
  url: string,
  body: Record<string, unknown>,
  alias: string,
) {
  cy.intercept('POST', url, {
    body: `)]}'\n${JSON.stringify(body)}`,
    headers: { 'X-Prpc-Grpc-Code': '0' },
  }).as(alias);
}
