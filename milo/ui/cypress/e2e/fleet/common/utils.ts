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

import { FleetConsoleMockAPI } from '../../../../src/fleet/testing_tools/mock_api';

/**
 * Mocks a pRPC endpoint in Cypress using schema-compliant FleetConsoleMockAPI fixtures.
 * Dynamically evaluates functional fixtures and gRPC status code error responses.
 * @param method The pRPC method name (e.g., 'ListDevices', 'CountDevices').
 * @param fixtureOverride Optional override payload or callback. If omitted, uses default schema fixture from FleetConsoleMockAPI.
 * @param alias The Cypress route alias (defaults to method name).
 */
export function mockPrpcEndpoint(
  method: string,
  fixtureOverride?: unknown,
  alias?: string,
) {
  const routeAlias = alias || method;

  cy.intercept('POST', `**/prpc/*/${method}*`, (req) => {
    let data =
      fixtureOverride !== undefined
        ? fixtureOverride
        : FleetConsoleMockAPI.getFixture(method);

    if (typeof data === 'function') {
      let payload = {};
      try {
        payload =
          typeof req.body === 'string' ? JSON.parse(req.body) : req.body || {};
      } catch {
        // Ignore
      }
      data = data(payload);
    }

    if (
      data &&
      typeof data === 'object' &&
      (data as Record<string, unknown>).__isError
    ) {
      const errObj = data as Record<string, unknown>;
      const grpcCode = String(errObj.grpcCode ?? 13);
      const message = String(errObj.message ?? 'pRPC Server Error');
      req.reply({
        statusCode: 500,
        headers: {
          'Content-Type': 'application/json',
          'X-Prpc-Grpc-Code': grpcCode,
        },
        body: ")]}'\n" + JSON.stringify({ message }),
      });
      return;
    }

    req.reply({
      statusCode: 200,
      headers: {
        'Content-Type': 'application/json',
        'X-Prpc-Grpc-Code': '0',
      },
      body: ")]}'\n" + JSON.stringify(data ?? {}),
    });
  }).as(routeAlias);
}

/**
 * Mocks authentication state in Cypress tests using FleetConsoleMockAPI.
 * @param authStateOverride Optional override for OpenID auth state.
 */
export function mockCypressAuth(authStateOverride?: Record<string, unknown>) {
  const authState = {
    identity: 'user:user@google.com',
    email: 'user@google.com',
    ...authStateOverride,
  };
  cy.intercept('GET', '**/auth/openid/state', {
    body: authState,
  }).as('authState');
}

/**
 * Legacy pRPC mocking helper for Cypress (retained for custom URL patterns).
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
