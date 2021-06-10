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
   */
  readonly matchHeaders?: readonly string[];
}

interface CachedRequest {
  readonly url: string;
  readonly method: string;
  readonly headers: { readonly [key: string]: string };
  readonly body: string;
}

interface CachedResponse {
  readonly statusCode: number;
  readonly headers: { readonly [key: string]: string };
  readonly body: string;
}

interface CacheEntry {
  readonly req: CachedRequest;
  readonly res: CachedResponse;
}

/**
 * Map filename -> cache entries.
 */
// A global variable should work because cypress init the module for each spec
// separately.
let cacheMap: Map<string, CacheEntry[]>;

/**
 * Converts a request/response body to a string.
 */
function serializeBody(body: unknown): string {
  return body instanceof Buffer || typeof body === 'string' ? body.toString() : JSON.stringify(body);
}

/**
 * Stubs all requests matched the routeMatcher.
 */
function stubRequests(routeMatcher: RouteMatcher, cacheName: string, opt: StubRequestsOption = {}) {
  // Use '.txt' instead of '.json' because the file might be empty (therefore
  // invalid) when first created.
  const filename = path.join('cypress', 'http_cache', Cypress.spec.name, cacheName + '.txt');
  const matchHeaders = opt.matchHeaders?.map((key) => key.toLowerCase());
  const stripIgnoredHeaders = (req: CachedRequest): CachedRequest => {
    if (!matchHeaders) {
      return req;
    }
    return {
      ...req,
      headers: Object.fromEntries(matchHeaders.map((key) => [key, req.headers[key]])),
    };
  };

  // Ensure the file exist so cy.readFile won't throw.
  cy.writeFile(filename, '', { flag: 'a+' });

  cy.readFile(filename).then((content) => {
    const cache = cacheMap.get(filename) || (JSON.parse(content || '[]') as CacheEntry[]);
    cy.intercept(routeMatcher, (incomingReq) => {
      // We need 2 request objects because
      // 1. we need to strip away ignored headers to perform cache matching.
      // 2. we need to store the full request in the cache in case the caller
      // decides to change the matched header.
      const fullReq: CachedRequest = {
        url: incomingReq.url,
        method: incomingReq.method,
        headers: incomingReq.headers,
        body: serializeBody(incomingReq.body),
      };
      const reqForComparison = stripIgnoredHeaders(fullReq);

      const res = cache.find((entry) => deepEqual(stripIgnoredHeaders(entry.req), reqForComparison))?.res;

      if (res) {
        incomingReq.reply(res.statusCode, res.body, res.headers);
      } else {
        incomingReq.on('after:response', (incomingRes) => {
          cache.push({
            req: fullReq,
            res: {
              statusCode: incomingRes.statusCode,
              // Content is already decoded when it hits Cypress.
              headers: { ...incomingRes.headers, 'content-encoding': '' },
              body: serializeBody(incomingRes.body),
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
