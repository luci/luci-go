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
 * Similar to `jest.createMockFromModule` but only mock the export keys
 * specified in `keysToMock`.
 */
declare function createSelectiveMockFromModule<T = unknown>(
  moduleName: string,
  keysToMock: ReadonlyArray<keyof NoInfer<T>>
): T;

/**
 * Spy the exported functions specified in `keysToSpy` in a module.
 *
 * The spy is created via `jest.fn(actualImpl)` instead of `jest.spyOn` so
 * `jest.restoreAllMocks()` will NOT restore the spies. This is intentional.
 * `createSelectiveSpiesFromModule` is usually called in `jest.mock`, which
 * usually lives in the top level scope of a test file, outside of a `before`
 * hook. Restoring the spies will break the subsequent unit tests in the test
 * file.
 */
declare function createSelectiveSpiesFromModule<T = unknown>(
  moduleName: string,
  keysToSpy: ReadonlyArray<FunctionKeys<NoInfer<T>>>
): T;
