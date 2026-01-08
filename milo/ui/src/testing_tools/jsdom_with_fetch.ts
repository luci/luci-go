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

import JSDOMEnvironment from 'jest-environment-jsdom';

/**
 * A custom Jest environment that patches JSDOM with the native Node.js Fetch API.
 *
 * JSDOM does not support the Fetch API by default. This environment copies the
 * native `fetch`, `Headers`, `Request`, and `Response` implementations from the
 * Node.js global scope into the JSDOM global scope.
 *
 * This allows tests to use `fetch` as if it were running in a browser environment
 * that supports it, without needing polyfills like `whatwg-fetch`.
 */
export default class JSDOMWithFetchEnvironment extends JSDOMEnvironment {
  async setup() {
    await super.setup();
    if (typeof fetch !== 'undefined') {
      this.global.fetch = fetch;
      this.global.Headers = Headers;
      this.global.Request = Request;
      this.global.Response = Response;
    } else {
      // eslint-disable-next-line no-console
      console.warn(
        'Node global fetch not found in JSDOMWithFetchEnvironment setup',
      );
    }
  }
}
