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

import { RouteMatcher } from 'cypress/types/net-stubbing';
import { deepEqual } from 'fast-equals';
import * as path from 'path';

import { BodyRepresentation, fromBodyRep, toBodyRep } from './serde';

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace Cypress {
    interface Chainable {
      /**
       * Stubs all HTTP requests matched the routeMatcher. HTTP requests that
       * completed during the execution of the test suite will be cached in
       * $project_root/cypress/http_cache/$spec_name/$cacheName.
       *
       * To ensure all requests are completed (and therefore cached), you can
       * set env variable REQUEST_CAPTURE_WINDOW to extend the duration of each
       * test case. (e.g. `cypress run --env REQUEST_CAPTURE_WINDOW=4000`
       * extends each test case by 4000ms).
       */
      stubRequests: typeof stubRequests;
    }
  }
}

export interface StubRequestsOption {
  /**
   * A list of headers that must match when resolving cache. If not specified,
   * all headers must match.
   *
   * If not specified, all headers need to be matched.
   */
  readonly matchHeaders?: readonly string[];
  /**
   * A function that determines whether the cached request matches the incoming
   * request. Headers not specified in matchHeaders are stripped from the
   * request before executing this function. The original headers as passed as
   * the 3rd an 4th parameters to the function.
   *
   * Defaults to deepEqual from 'fast-equals'.
   */
  readonly matchRequest?: (
    cached: CachedRequest<BodyRepresentation>,
    incoming: CachedRequest<BodyRepresentation>,
    cachedHeaders: { readonly [key: string]: string },
    incomingHeaders: { readonly [key: string]: string }
  ) => boolean;
}

interface CachedRequest<Body> {
  readonly url: string;
  readonly method: string;
  readonly headers: { readonly [key: string]: string };
  readonly body: Body;
}

interface CachedResponse {
  readonly statusCode: number;
  readonly headers: { readonly [key: string]: string };
  readonly body: BodyRepresentation;
}

interface CacheEntry {
  readonly req: CachedRequest<BodyRepresentation>;
  readonly res: CachedResponse;
}

/**
 * Converts header keys with multiple values from using array representation to
 * using string representation.
 */
function normalizeHeaders(headers: { [key: string]: string | string[] }): { [key: string]: string } {
  return Object.fromEntries(Object.entries(headers).map(([k, v]) => [k, Array.isArray(v) ? v.sort().join(', ') : v]));
}

/**
 * Map filename -> cache entries.
 */
// A global variable should work because cypress init the module for each spec
// separately.
let cacheMap: Map<string, CacheEntry[]>;

/**
 * Stubs all requests matched the routeMatcher.
 */
function stubRequests(routeMatcher: RouteMatcher, cacheName: string, opt: StubRequestsOption = {}) {
  // Use '.txt' instead of '.json' because the file might be empty (therefore
  // invalid) when first created.
  const filename = path.join('cypress', 'http_cache', Cypress.spec.name, cacheName + '.txt');
  const matchHeaders = opt.matchHeaders?.map((key) => key.toLowerCase());

  function stripIgnoredHeaders<T>(req: CachedRequest<T>): CachedRequest<T> {
    if (!matchHeaders) {
      return req;
    }
    return {
      ...req,
      headers: Object.fromEntries(matchHeaders.map((key) => [key, req.headers[key]])),
    };
  }

  const matchRequest = opt.matchRequest || deepEqual;

  // Ensure the file exist so cy.readFile won't throw.
  cy.writeFile(filename, '', { flag: 'a+' });

  cy.readFile(filename).then((content) => {
    const cache = cacheMap.get(filename) || (JSON.parse(content || '[]') as CacheEntry[]);
    cy.intercept(routeMatcher, (incomingReq) => {
      // We need 2 request objects because
      // 1. we need to strip away ignored headers to perform cache matching.
      // 2. we need to store the full request in the cache in case the caller
      // decides to change the matched header.
      const fullReq: CachedRequest<BodyRepresentation> = {
        url: incomingReq.url,
        method: incomingReq.method,
        headers: normalizeHeaders(incomingReq.headers),
        body: toBodyRep(incomingReq.body),
      };

      const res = cache.find((entry) =>
        matchRequest(stripIgnoredHeaders(entry.req), stripIgnoredHeaders(fullReq), entry.req.headers, fullReq.headers)
      )?.res;

      if (res) {
        incomingReq.reply(res.statusCode, fromBodyRep(res.body), res.headers);
      } else {
        incomingReq.on('after:response', (incomingRes) => {
          cache.push({
            req: fullReq,
            res: {
              statusCode: incomingRes.statusCode,
              // Content is already decoded when it hits Cypress.
              headers: { ...incomingRes.headers, 'content-encoding': '' },
              body: toBodyRep(incomingRes.body),
            },
          });
          cacheMap.set(filename, cache);
        });
      }
    });
  });
}

/**
 * Adds stubRequests command to Cypress.
 */
export function addStubRequestsCommand() {
  Cypress.Commands.add('stubRequests', stubRequests);

  before(() => {
    cacheMap = new Map<string, CacheEntry[]>();
  });

  // Some HTTP requests might not be cached if they did not finish before the
  // test case ends. Extending the test case reduce flakiness caused by unstable
  // network condition.
  const captureWindow = Number(Cypress.env('REQUEST_CAPTURE_WINDOW')) || 0;
  afterEach(() => {
    cy.wait(captureWindow);
  });

  // Update files after the test suite finished.
  // We can't update the files when test suites are running because:
  // 1. Cypress tasks need to be scheduled sequentially.
  // 2. The response may comeback when a cypress task is running.
  after(() => {
    for (const [filename, entries] of cacheMap) {
      cy.writeFile(filename, JSON.stringify(entries, undefined, 2));
    }
  });
}
