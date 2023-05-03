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

import { Outlet, useLoaderData } from 'react-router-dom';

import { AuthStateProvider } from '../components/auth_state_provider';
import { ErrorBoundary } from '../components/error_boundary';
import { TopBar } from '../components/top_bar';
import { AuthState } from '../libs/auth_state';

/**
 * Renders page header, and tooltip.
 */
export function BaseLayout() {
  const initialAuthState = useLoaderData() as AuthState;

  return (
    <ErrorBoundary>
      <AuthStateProvider initialValue={initialAuthState}>
        <TopBar />
        {/*
         * <TopBar /> supports useful actions in case of an error (e.g. file a
         * bug, log in/out).
         * Wraps <Outlet /> in a separate <ErrorBoundary /> to ensure the
         * <TopBar /> is always displayed when possible.
         *
         */}
        <ErrorBoundary>
          <Outlet />
        </ErrorBoundary>
      </AuthStateProvider>
    </ErrorBoundary>
  );
}
