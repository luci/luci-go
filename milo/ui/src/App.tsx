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

import '@/common/styles/common_style.css';
import '@/common/styles/color_classes.css';
import '@/common/components/tooltip';

import { GrpcError } from '@chopsui/prpc-client';
import { ThemeProvider } from '@emotion/react';
import { LocalizationProvider } from '@mui/x-date-pickers';
import { AdapterLuxon } from '@mui/x-date-pickers/AdapterLuxon';
import {
  QueryClient,
  QueryClientConfig,
  QueryClientProvider,
} from '@tanstack/react-query';
import { destroy } from 'mobx-state-tree';
import { useEffect, useState } from 'react';
import { createBrowserRouter, RouterProvider } from 'react-router';

import releaseNotes from '@root/RELEASE_NOTES.md?raw';

import { obtainAuthState } from '@/common/api/auth_state';
import { AuthStateInitializer } from '@/common/components/auth_state_provider';
import {
  RecoverableErrorBoundary,
  RouteErrorDisplay,
} from '@/common/components/error_handling';
import { LitEnvProvider } from '@/common/components/lit_env_provider';
import { PageConfigStateProvider } from '@/common/components/page_config_state_provider';
import { PageMetaProvider } from '@/common/components/page_meta';
import { VersionControlProvider } from '@/common/components/version_control/version_control_provider';
import { NON_TRANSIENT_ERROR_CODES } from '@/common/constants/rpc';
import { FeatureFlagsProvider } from '@/common/feature_flags/provider';
import { BaseLayout } from '@/common/layouts/base_layout';
import { Store, StoreProvider } from '@/common/store';
import { theme } from '@/common/themes/base';
import { ReleaseNotesProvider } from '@/core/components/release_notes';
import { parseReleaseNotes } from '@/core/components/release_notes/common';
import { routes } from '@/core/routes';
import { ReactLitBridge } from '@/generic_libs/components/react_lit_element';
import { useIsDevBuild } from '@/generic_libs/hooks/is_dev_build';
import { SingletonStoreProvider } from '@/generic_libs/hooks/singleton';
import { SyncedSearchParamsProvider } from '@/generic_libs/hooks/synced_search_params';

const isNonTransientError = (error: unknown) =>
  error instanceof GrpcError && NON_TRANSIENT_ERROR_CODES.includes(error.code);

const QUERY_CLIENT_CONFIG: QueryClientConfig = {
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => {
        // Do not retry when the errors is non-transient.
        if (isNonTransientError(error)) {
          return false;
        }

        // Keep the default retry behavior otherwise.
        return failureCount < 3;
      },
      refetchOnWindowFocus(query) {
        // Do not refetch when there was an error.
        //
        // Components often occupy vastly different amount of screen space when
        // the query is loading comparing to when the query succeeded/failed.
        // Refetching a failed query (therefore no stale data is available) that
        // is destined to fail again can cause page-shifting with no benefit.
        //
        // We do not attempt to check whether the error is non-transient here.
        // If the query did not succeeded during normal retries, it's very
        // unlikely to succeed now.
        if (query.state.error) {
          return false;
        }
        // Keep the default refetch behavior otherwise.
        return true;
      },
    },
  },
};

export function App() {
  const isDevBuild = useIsDevBuild();
  const [store] = useState(() => Store.create({}));
  const [queryClient] = useState(() => new QueryClient(QUERY_CLIENT_CONFIG));

  useEffect(
    () => {
      // Expose `store` in the global namespace to make inspecting/debugging the
      // store via the browser dev-tool easier.
      //
      // The __STORE variable should only be used for debugging purpose. As
      // such, do not declare __STORE as a global variable explicity so it's
      // less likely to be misused.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (window as any).__STORE = store;

      return () => destroy(store);
    },
    // None of them will ever change. But list them as dependencies to make
    // ESLint happy.
    [store, isDevBuild],
  );

  const router = createBrowserRouter([
    {
      // Use a 'ui' route to enclose all routes instead of setting the basename
      // to 'ui' so the URLs work the same whether they are consumed by a
      // component/function imported from 'react-router' or from other modules.
      path: 'ui',
      loader: async () => obtainAuthState(),
      element: (
        <SyncedSearchParamsProvider>
          <AuthStateInitializer>
            <FeatureFlagsProvider>
              <RecoverableErrorBoundary>
                <ReactLitBridge>
                  <milo-tooltip />
                  <BaseLayout />
                </ReactLitBridge>
              </RecoverableErrorBoundary>
            </FeatureFlagsProvider>
          </AuthStateInitializer>
        </SyncedSearchParamsProvider>
      ),
      // Catch the errors that may happen in various providers.
      // Cannot use `<RecoverableErrorBoundary />` here because it requires the
      // auth state provider.
      errorElement: <RouteErrorDisplay />,
      children: [
        ...routes,
        {
          path: '*',
          lazy: () => import('@/core/pages/not_found_page'),
        },
      ],
    },
    {
      // We don't have a basename to stop react-router from handling non-SPA
      // routes (see the comments on the 'ui' route for rationale). We need to
      // to capture those routes and make the server handles it.
      path: '*',
      lazy: () => import('@/core/pages/server_page'),
    },
  ]);

  // As a rule of thumb, we should put our own providers in inner layers since
  // they have a chance of depending on contexts provided by 3rd party providers
  // while 3rd party providers have no chance depending on our providers (with
  // the exception of <RouterProvider /> since the pages depend on the
  // providers).
  return (
    <LocalizationProvider dateAdapter={AdapterLuxon}>
      <ThemeProvider theme={theme}>
        <QueryClientProvider client={queryClient}>
          <SingletonStoreProvider>
            <VersionControlProvider>
              <StoreProvider value={store}>
                <LitEnvProvider>
                  <PageMetaProvider>
                    <ReleaseNotesProvider
                      initReleaseNotes={parseReleaseNotes(releaseNotes)}
                    >
                      <PageConfigStateProvider>
                        <RouterProvider router={router} />
                      </PageConfigStateProvider>
                    </ReleaseNotesProvider>
                  </PageMetaProvider>
                </LitEnvProvider>
              </StoreProvider>
            </VersionControlProvider>
          </SingletonStoreProvider>
        </QueryClientProvider>
      </ThemeProvider>
    </LocalizationProvider>
  );
}
