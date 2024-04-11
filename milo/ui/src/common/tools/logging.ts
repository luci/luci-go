// Copyright 2023 The LUCI Authors.
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
 * Similar to `self.console` except that
 * 1. only a limited set of methods are available, and
 * 2. `.log` is not available to encourage displaying message via DOM, and
 * 3. logging can easily be disabled/intercepted in tests using `jest.spyOn`
 *    without affecting logs from 3rd party libraries.
 */
// Do not use `console.warn.bind(console)` otherwise mocking `console.warn` will
// not have an effect on `logging.warn`. The same applies to `console.error`.
export const logging = {
  // eslint-disable-next-line no-console
  warn: (...params: Parameters<typeof console.warn>) => console.warn(...params),
  error: (...params: Parameters<typeof console.error>) =>
    // eslint-disable-next-line no-console
    console.error(...params),
};
