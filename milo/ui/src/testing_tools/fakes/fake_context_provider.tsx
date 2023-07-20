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

import { LocalizationProvider } from '@mui/x-date-pickers';
import { AdapterLuxon } from '@mui/x-date-pickers/AdapterLuxon';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React, { useState } from 'react';
import { RouterProvider, createMemoryRouter } from 'react-router-dom';

import { FakeAuthStateProvider } from './fake_auth_state_provider';

interface FakeContextProviderProps {
  readonly mountedPath?: string;
  readonly routerOptions?: Parameters<typeof createMemoryRouter>[1];
  readonly children: React.ReactNode;
}

/**
 * Provides various contexts for testing purpose.
 */
export function FakeContextProvider({
  mountedPath = '/',
  routerOptions,
  children,
}: FakeContextProviderProps) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            retry: false,
          },
        },
      })
  );

  const router = createMemoryRouter(
    [{ path: mountedPath, element: children }],
    routerOptions
  );

  return (
    <LocalizationProvider dateAdapter={AdapterLuxon}>
      <FakeAuthStateProvider>
        <QueryClientProvider client={client}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </FakeAuthStateProvider>
    </LocalizationProvider>
  );
}
