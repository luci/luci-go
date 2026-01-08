// Copyright 2025 The LUCI Authors.
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

import * as path from 'path';

import callsites from 'callsites';

import { FunctionKeys } from '@/generic_libs/types';

/**
 * Resolves a module in the specified file.
 */
function resolveModuleInFile(moduleName: string, filename: string): string {
  if (!moduleName.startsWith('.')) {
    return moduleName;
  }

  return path.join(path.dirname(filename), moduleName);
}

/**
 * See the type definition for `self.createSelectiveMockFromModule`.
 */
export function createSelectiveMockFromModule<T = unknown>(
  moduleName: string,
  keysToMock: ReadonlyArray<keyof NoInfer<T>>,
): T {
  // Resolve the module at the caller site.
  // This allows the caller to mock a module with a relative path.
  moduleName = resolveModuleInFile(moduleName, callsites()[1].getFileName()!);
  const actualModule = jest.requireActual(moduleName);
  const mockedModule = jest.createMockFromModule(moduleName) as T;

  return {
    ...actualModule,
    ...Object.fromEntries(keysToMock.map((k) => [k, mockedModule[k]])),
  };
}

/**
 * See the type definition for `self.createSelectiveSpiesFromModule`.
 */
export function createSelectiveSpiesFromModule<T = unknown>(
  moduleName: string,
  keysToSpy: ReadonlyArray<FunctionKeys<NoInfer<T>>>,
): T {
  // Resolve the module at the caller site.
  // This allows the caller to mock a module with a relative path.
  moduleName = resolveModuleInFile(moduleName, callsites()[1].getFileName()!);
  const actualModule = jest.requireActual(moduleName);
  return {
    ...actualModule,
    ...Object.fromEntries(keysToSpy.map((k) => [k, jest.fn(actualModule[k])])),
  };
}

/**
 * Manages fetch mocks for Jest tests.
 *
 * This class allows registering multiple handlers for `fetch`.
 * Handlers are checked in LIFO (Last-In-First-Out) order, meaning the most recently
 * added handler that matches a request will check it first.
 *
 * It automatically spies on `global.fetch` when the first handler is added.
 * Call `MockFetchManager.clear()` (or `resetMockFetch()`) in `afterEach` to clean up.
 */
export class MockFetchManager {
  private static handlers: Array<{
    pred: (url: string, init?: RequestInit) => boolean;
    handler: (url: string, init?: RequestInit) => Promise<Response>;
  }> = [];
  private static spy: jest.SpyInstance | null = null;

  /**
   * Adds a new fetch handler.
   *
   * @param pred A predicate function that returns true if the handler should handle the request.
   * @param handler The handler function that returns a Promise resolving to a Response.
   */
  static addHandler(
    pred: (url: string, init?: RequestInit) => boolean,
    handler: (url: string, init?: RequestInit) => Promise<Response>,
  ) {
    this.ensureSpy();
    this.handlers.push({ pred, handler });
  }

  /**
   * Clears all registered handlers and restores the original `fetch`.
   */
  static clear() {
    this.handlers = [];
    if (this.spy) {
      this.spy.mockRestore();
      this.spy = null;
    }
  }

  private static ensureSpy() {
    if (this.spy) return;
    this.spy = jest
      .spyOn(global, 'fetch')
      .mockImplementation(async (input, init) => {
        const url = input.toString();
        // Iterate in reverse (LIFO) so that later mocks override earlier ones.
        for (let i = this.handlers.length - 1; i >= 0; i--) {
          const { pred, handler } = this.handlers[i];
          if (pred(url, init)) {
            return handler(url, init);
          }
        }
        // eslint-disable-next-line no-console
        console.warn(`[MockFetch] Unhandled fetch: ${url}`);
        return new Response('Not Found', { status: 404 });
      });
  }
}

/**
 * Clean up mocks after tests. call this in afterEach if needed, or rely on manual clear.
 * Ideally, add this to setup_after_env.ts `afterEach`.
 */
export const resetMockFetch = () => MockFetchManager.clear();

/**
 * Mocks a specific URL or predicate with a JSON response.
 */
export const mockFetchMatch = <T>(
  matcher: string | RegExp | ((url: string, init?: RequestInit) => boolean),
  data: T,
  status = 200,
) => {
  const pred =
    typeof matcher === 'function'
      ? matcher
      : typeof matcher === 'string'
        ? (url: string) => url.includes(matcher)
        : (url: string) => matcher.test(url);

  MockFetchManager.addHandler(pred, async () => {
    return new Response(JSON.stringify(data), {
      status,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      headers: { 'Content-Type': 'application/json' } as any,
    });
  });
};

/**
 * Mocks a specific URL or predicate with a raw response body.
 */
export const mockFetchRaw = (
  matcher: string | RegExp | ((url: string, init?: RequestInit) => boolean),
  body: BodyInit,
  init?: ResponseInit,
) => {
  const pred =
    typeof matcher === 'function'
      ? matcher
      : typeof matcher === 'string'
        ? (url: string) => url.includes(matcher)
        : (url: string) => matcher.test(url);

  MockFetchManager.addHandler(pred, async () => {
    return new Response(body, init);
  });
};

/**
 * Mocks a specific URL or predicate with a custom handler.
 */
export const mockFetchHandler = (
  matcher: string | RegExp | ((url: string, init?: RequestInit) => boolean),
  handler: (url: string, init?: RequestInit) => Promise<Response>,
) => {
  const pred =
    typeof matcher === 'function'
      ? matcher
      : typeof matcher === 'string'
        ? (url: string) => url.includes(matcher)
        : (url: string) => matcher.test(url);

  MockFetchManager.addHandler(pred, handler);
};

/**
 * Mocks the global fetch API to return a JSON response.
 * WARNING: This uses the shared MockFetchManager, so it plays nice with other mocks.
 * It adds a CATCH-ALL handler.
 */
/**
 * Mocks the global fetch API to return a JSON response.
 * This directly mocks the global fetch spy.
 */
export const mockFetchJson = <T>(data: T, status = 200) => {
  return jest.spyOn(global, 'fetch').mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: async () => data,
    text: async () => JSON.stringify(data),
    headers: new Headers({ 'Content-Type': 'application/json' }),
    blob: async () => new Blob([JSON.stringify(data)]),
    arrayBuffer: async () => new TextEncoder().encode(JSON.stringify(data)),
  } as unknown as Response);
};
