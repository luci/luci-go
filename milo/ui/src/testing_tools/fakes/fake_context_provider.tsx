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

import { ThemeProvider } from '@emotion/react';
import { LocalizationProvider } from '@mui/x-date-pickers';
import { AdapterLuxon } from '@mui/x-date-pickers/AdapterLuxon';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React, { useState } from 'react';
import { Outlet, RouterProvider, createMemoryRouter } from 'react-router-dom';

import { PageConfigStateProvider } from '@/common/components/page_config_state_provider';
import { PageMetaProvider } from '@/common/components/page_meta/page_meta_provider';
import { PermCheckProvider } from '@/common/components/perm_check_provider';
import { UiPage } from '@/common/constants/view';
import { theme } from '@/common/themes/base';
import { ReleaseNotesProvider } from '@/core/components/release_notes';
import { SyncedSearchParamsProvider } from '@/generic_libs/hooks/synced_search_params';

import { FakeAuthStateProvider } from './fake_auth_state_provider';

interface FakeContextProviderProps {
  readonly mountedPath?: string;
  readonly routerOptions?: Parameters<typeof createMemoryRouter>[1];
  readonly children: React.ReactNode;
  readonly pageMeta?: {
    project?: string;
    selectedPage?: UiPage;
  };
}

/**
 * Provides various contexts for testing purpose.
 */
export function FakeContextProvider({
  mountedPath,
  children,
  pageMeta,
  routerOptions,
}: FakeContextProviderProps) {
  const [client] = useState(() => {
    const errorMock = jest
      .spyOn(console, 'error')
      .mockImplementationOnce(() => {
        // Prevent react query from complaining about the custom logger.
        //
        // Custom logger is deprecated and will be removed in
        // `@tanstack/react-query@5`. However, we still need to use it here to
        // disable unnecessary network call error logs in unit tests.
        //
        // In the next react-query release, this will no longer be necessary
        // since network call errors will no longer be logged anyway. See the
        // `remove custom logger` section on
        // https://github.com/TanStack/query/discussions/4252
      });
    const c = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
      logger: {
        log: () => {},
        warn: () => {},
        error: () => {},
      },
    });
    errorMock.mockRestore();
    return c;
  });

  const router = createMemoryRouter(
    [
      {
        element: (
          <SyncedSearchParamsProvider>
            <Outlet />
          </SyncedSearchParamsProvider>
        ),
        children: [
          {
            path: mountedPath || '/',
            element: children,
          },
        ],
      },
    ],
    routerOptions,
  );

  return (
    <ThemeProvider theme={theme}>
      <LocalizationProvider dateAdapter={AdapterLuxon}>
        <FakeAuthStateProvider>
          <QueryClientProvider client={client}>
            <PermCheckProvider>
              <PageMetaProvider
                initPage={pageMeta?.selectedPage}
                initProject={pageMeta?.project}
              >
                <ReleaseNotesProvider
                  initReleaseNotes={{ latest: '', latestVersion: -1, past: '' }}
                >
                  <PageConfigStateProvider>
                    <RouterProvider router={router} />
                  </PageConfigStateProvider>
                </ReleaseNotesProvider>
              </PageMetaProvider>
            </PermCheckProvider>
          </QueryClientProvider>
        </FakeAuthStateProvider>
      </LocalizationProvider>
    </ThemeProvider>
  );
}
