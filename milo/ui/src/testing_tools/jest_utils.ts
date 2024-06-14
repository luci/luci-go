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
