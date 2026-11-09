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
import { createHashRouter, redirect, RouterProvider } from 'react-router';

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

import {
  installPrototypeNavigationInterceptor,
  resolveInitialPrototypeUrl,
} from './prototype_routing';

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

interface PrototypeWindow extends Window {
  __FCON_INITIAL_ROUTE__?: string;
  __FCON_LIVE_DATA__?: boolean;
  __FCON_LIVE_AUTH_CTX__?: AuthStateContextValue;
}

/**
 * Returns true when the prototype bundle is running in opt-in live-data mode
 * (`--live-data` build flag or `?fcon_data=live` URL parameter).
 */
function isLiveDataPrototypeMode(): boolean {
  if (typeof window === 'undefined') return false;
  const params = new URLSearchParams(window.location.search);
  const fconData = params.get('fcon_data');
  if (fconData === 'live') return true;
  if (fconData === 'mock') return false;
  return Boolean((window as PrototypeWindow).__FCON_LIVE_DATA__);
}

/**
 * Returns the active `AuthStateContextValue` for the prototype shell, delegating
 * to host-injected live OAuth credentials when running in live-data mode.
 */
function getPrototypeAuthContext(): AuthStateContextValue {
  if (typeof window !== 'undefined' && isLiveDataPrototypeMode()) {
    const injected = (window as PrototypeWindow).__FCON_LIVE_AUTH_CTX__;
    if (injected) return injected;
  }
  return MOCK_AUTH_CTX;
}

/**
 * Renders the standalone Fleet Console application shell for offline prototype
 * bundles using hash routing (`#/ui/fleet/...`) and intercepting internal links.
 */
export function App(): JSX.Element {
  const [queryClient] = useState(
    () => new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  );

  const liveDataMode = isLiveDataPrototypeMode();
  if (!liveDataMode && !FleetConsoleMockAPI.isPrototypeMode()) {
    FleetConsoleMockAPI.initPrototypeMode({ authState: MOCK_AUTH_STATE });
  }
  const authCtx = getPrototypeAuthContext();

  const [router] = useState(() => {
    const initialRoute = getInitialRoute();
    return createHashRouter(
      [
        { index: true, loader: () => redirect(initialRoute) },
        {
          path: 'ui',
          handle: { appId: 'milo' },
          HydrateFallback: () => null,
          element: (
            <SyncedSearchParamsProvider>
              <AuthStateContext.Provider value={authCtx}>
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
      { window },
    );
  });

  useEffect(() => installPrototypeNavigationInterceptor(window), []);

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
  const configuredRoute = (window as { __FCON_INITIAL_ROUTE__?: string })
    .__FCON_INITIAL_ROUTE__;
  const res = resolveInitialPrototypeUrl(location.href, '', configuredRoute);
  history.replaceState(null, '', res.cleanHref);
  return res.route;
}
