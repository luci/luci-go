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

export * from './auth_state_initializer';
export { AuthStateProvider } from './auth_state_provider';
export type { AuthStateProviderProps } from './auth_state_provider';
export * from './constants';
export { useAuthState, useGetAccessToken, useGetIdToken } from './hooks';

import { AuthStateContext } from './auth_state_provider';

/**
 * Should only be used to override auth state context in tests.
 */
// Don't `export { AuthStateContext as TestAuthStateContext } from './context'`.
// Reassign and export to breaks IDE's find reference feature won't mix
// `AuthStateContext` and `TestAuthStateContext` together.
export const TestAuthStateContext = AuthStateContext;
