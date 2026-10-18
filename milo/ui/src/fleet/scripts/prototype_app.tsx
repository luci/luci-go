// Copyright 2026 The LUCI Authors.
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

import { ThemeProvider } from '@emotion/react';
import { LocalizationProvider } from '@mui/x-date-pickers';
import { AdapterLuxon } from '@mui/x-date-pickers/AdapterLuxon';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { JSX, useEffect, useState } from 'react';
import { createMemoryRouter, redirect, RouterProvider } from 'react-router';

import {
  AuthStateContext,
  AuthStateContextValue,
} from '@/common/components/auth_state_provider/context';
import {
  RecoverableErrorBoundary,
  RouteErrorDisplay,
} from '@/common/components/error_handling';
import { PageConfigStateProvider } from '@/common/components/page_config_state_provider';
import { PageMetaProvider } from '@/common/components/page_meta';
import { VersionControlProvider } from '@/common/components/version_control/version_control_provider';
import { FeatureFlagsProvider } from '@/common/feature_flags/provider';
import { theme } from '@/common/themes/base';
import { FleetLayout } from '@/fleet/layouts';
import { fleetRoutes } from '@/fleet/routes';
import { FleetConsoleMockAPI } from '@/fleet/testing_tools/mock_api';
import { SingletonStoreProvider } from '@/generic_libs/hooks/singleton';
import { SyncedSearchParamsProvider } from '@/generic_libs/hooks/synced_search_params';

const MOCK_AUTH_STATE = {
  email: 'user@google.com',
  identity: 'user:user@google.com',
  picture: '',
};

const MOCK_AUTH_CTX: AuthStateContextValue = {
  getAuthState: () => MOCK_AUTH_STATE,
  getAccessToken: () => Promise.resolve('prototype-mock-access-token'),
  getIdToken: () => Promise.resolve('prototype-mock-id-token'),
};

const DEFAULT_FLEET_ROUTE = '/ui/fleet/p/chromeos/devices';

/**
 * Renders the standalone Fleet Console application shell for offline prototype
 * bundles, syncing the in-memory router state with `window.location.hash`.
 */
export function App(): JSX.Element {
  const [queryClient] = useState(
    () => new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  );

  if (!FleetConsoleMockAPI.isPrototypeMode()) {
    FleetConsoleMockAPI.initPrototypeMode({ authState: MOCK_AUTH_STATE });
  }

  const initialRoute = getInitialRoute();
  const [router] = useState(() =>
    createMemoryRouter(
      [
        { index: true, loader: () => redirect(initialRoute) },
        {
          path: 'ui',
          handle: { appId: 'milo' },
          HydrateFallback: () => null,
          element: (
            <SyncedSearchParamsProvider>
              <AuthStateContext.Provider value={MOCK_AUTH_CTX}>
                <FeatureFlagsProvider>
                  <RecoverableErrorBoundary>
                    <FleetLayout />
                  </RecoverableErrorBoundary>
                </FeatureFlagsProvider>
              </AuthStateContext.Provider>
            </SyncedSearchParamsProvider>
          ),
          errorElement: <RouteErrorDisplay />,
          children: [
            {
              path: 'fleet',
              handle: { appId: 'fleet' },
              children: fleetRoutes,
            },
          ],
        },
        { path: '*', loader: () => redirect(initialRoute) },
      ],
      { initialEntries: [initialRoute] },
    ),
  );

  useEffect(() => {
    const unsubscribe = router.subscribe((state) => {
      const nextHash =
        '#' +
        state.location.pathname +
        state.location.search +
        state.location.hash;
      if (window.location.hash !== nextHash) {
        const url = new URL(window.location.href);
        url.hash = nextHash;
        window.history.replaceState(null, '', url.toString());
      }
    });
    const onHashChange = () => {
      const hashRoute = window.location.hash.slice(1);
      if (hashRoute.startsWith('/ui/')) {
        void router.navigate(hashRoute, { replace: true });
      }
    };
    window.addEventListener('hashchange', onHashChange);
    window.addEventListener('popstate', onHashChange);
    return () => {
      unsubscribe();
      window.removeEventListener('hashchange', onHashChange);
      window.removeEventListener('popstate', onHashChange);
    };
  }, [router]);

  return (
    <LocalizationProvider dateAdapter={AdapterLuxon}>
      <ThemeProvider theme={theme}>
        <QueryClientProvider client={queryClient}>
          <SingletonStoreProvider>
            <VersionControlProvider>
              <PageMetaProvider>
                <PageConfigStateProvider>
                  <RouterProvider router={router} />
                </PageConfigStateProvider>
              </PageMetaProvider>
            </VersionControlProvider>
          </SingletonStoreProvider>
        </QueryClientProvider>
      </ThemeProvider>
    </LocalizationProvider>
  );
}

function getInitialRoute(): string {
  const hashRoute = window.location.hash.slice(1);
  if (hashRoute.startsWith('/ui/')) {
    return hashRoute;
  }
  const configuredRoute = (window as { __FCON_INITIAL_ROUTE__?: string })
    .__FCON_INITIAL_ROUTE__;
  return configuredRoute && configuredRoute.startsWith('/ui/')
    ? configuredRoute
    : DEFAULT_FLEET_ROUTE;
}
