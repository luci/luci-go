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

interface CachedRequest {
  url: string;
  method: string;
  headers: { [key: string]: string };
  body: string;
}

interface CachedResponse {
  statusCode: number;
  headers: { [key: string]: string };
  body: string;
}

interface CacheEntry {
  req: CachedRequest;
  res: CachedResponse;
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
function stubRequests(routeMatcher: RouteMatcher, cacheName: string) {
  // Use '.txt' instead of '.json' because the file might be empty (therefore
  // invalid) when first created.
  const filename = path.join('cypress', 'http_cache', Cypress.spec.name, cacheName + '.txt');

  // Ensure the file exist so cy.readFile won't throw.
  cy.writeFile(filename, '', { flag: 'a+' });

  cy.readFile(filename).then((content) => {
    const cache = cacheMap.get(filename) || (JSON.parse(content || '[]') as CacheEntry[]);
    cy.intercept(routeMatcher, (incomingReq) => {
      const req: CachedRequest = {
        url: incomingReq.url,
        method: incomingReq.method,
        headers: incomingReq.headers,
        body: serializeBody(incomingReq.body),
      };
      const res = cache.find((entry) => deepEqual(entry.req, req))?.res;
      if (res) {
        incomingReq.reply(res.statusCode, res.body, res.headers);
      } else {
        incomingReq.on('after:response', (incomingRes) => {
          cache.push({
            req,
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
export function addStubRequestCommand() {
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
