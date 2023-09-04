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
import { useEffect, useRef, useState } from 'react';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { Workbox } from 'workbox-window';

import '@/common/styles/common_style.css';
import '@/common/styles/color_classes.css';
import '@/common/components/tooltip';
import { BisectionLayout } from '@/bisection/layouts/base';
import { AnalysisDetailsPage } from '@/bisection/pages/analysis_details';
import { FailureAnalysesPage } from '@/bisection/pages/failure_analyses';
import { obtainAuthState } from '@/common/api/auth_state';
import { LitEnvProvider } from '@/common/components/lit_env_provider';
import { BaseLayout } from '@/common/layouts/base_layout';
import { Store, StoreProvider } from '@/common/store';
import { theme } from '@/common/themes/base';
import { createStaticTrustedURL } from '@/generic_libs/tools/utils';

import { AuthStateInitializer } from './common/components/auth_state_provider';
import { RecoverableErrorBoundary } from './common/components/error_handling';
import { RouteErrorDisplay } from './common/components/error_handling/route_error_display';
import { PageMetaProvider } from './common/components/page_meta/page_meta_provider';
import { NON_TRANSIENT_ERROR_CODES } from './common/constants';
import { SyncedSearchParamsProvider } from './generic_libs/hooks/synced_search_params';
import { ArtifactPageLayout } from './pages/artifact/artifact_page_layout';
import { ImageDiffArtifactPage } from './pages/artifact/image_diff_artifact_page';
import { RawArtifactPage } from './pages/artifact/raw_artifact_page';
import { TextDiffArtifactPage } from './pages/artifact/text_diff_artifact_page';
import { BuildPage } from './pages/build_page';
import { BlamelistTab } from './pages/build_page/blamelist_tab';
import { BuildDefaultTab } from './pages/build_page/build_default_tab';
import { BuildPageShortLink } from './pages/build_page/build_page_short_link';
import { BuildPageTab } from './pages/build_page/common';
import { OverviewTab } from './pages/build_page/overview_tab/overview_tab';
import { RelatedBuildsTab } from './pages/build_page/related_builds_tab';
import { StepsTab } from './pages/build_page/steps_tab';
import { TimelineTab } from './pages/build_page/timeline_tab';
import { BuilderPage } from './pages/builder_page';
import { BuildersPage } from './pages/builders_page/builders';
import { InvocationDefaultTab } from './pages/invocation_page/invocation_default_tab';
import { InvocationDetailsTab } from './pages/invocation_page/invocation_details_tab';
import { InvocationPage } from './pages/invocation_page/invocation_page';
import { LoginPage } from './pages/login_page';
import { NotFoundPage } from './pages/not_found_page';
import { searchRedirectionLoader } from './pages/search';
import { BuilderSearch } from './pages/search/builder_search';
import { TestSearch } from './pages/search/test_search/test_search';
import { ServerPage } from './pages/server_page';
import { TestHistoryPage } from './pages/test_history_page/test_history_page';
import { TestResultsTab } from './pages/test_results_tab/test_results_tab';
import { SwarmingBuildPage } from './swarming/views/swarming_build_page';

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
        // Do not refetch when the errors is non-transient.
        //
        // Components often occupy vastly different amount of screen space when
        // the query is loading comparing to when the query succeeded/failed.
        // Refetching a failed query (therefore no stale data is available) that
        // is destined to fail again can cause page-shifting with no benefit.
        if (isNonTransientError(query.state.error)) {
          return false;
        }
        // Keep the default refetch behavior otherwise.
        return true;
      },
    },
  },
};

export interface AppProps {
  /**
   * The App's configuration. The value is only used when initializing the App.
   * Updates are not applied.
   */
  readonly initOpts: {
    readonly isDevEnv: boolean;
    readonly enableUiSW: boolean;
  };
}

export function App({ initOpts }: AppProps) {
  const firstInitOpts = useRef(initOpts);
  const [store] = useState(() =>
    Store.create({}, { isDevEnv: firstInitOpts.current.isDevEnv }),
  );
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

      const { isDevEnv, enableUiSW } = firstInitOpts.current;

      if (navigator.serviceWorker && enableUiSW) {
        // vite-plugin-pwa hosts the service worker in a different route in dev
        // mode.
        // See https://vite-pwa-org.netlify.app/guide/development.html#injectmanifest-strategy
        const uiSwUrl = isDevEnv ? '/ui/dev-sw.js?dev-sw' : '/ui/ui_sw.js';
        const workbox = new Workbox(
          createStaticTrustedURL('sw-js-static', uiSwUrl),
          { type: isDevEnv ? 'module' : 'classic' },
        );
        workbox.register();
      }
      if (
        navigator.serviceWorker &&
        !document.cookie.includes('showNewBuildPage=false')
      ) {
        navigator.serviceWorker
          .register(
            // cast to string because TypeScript doesn't allow us to use
            // TrustedScriptURL here
            createStaticTrustedURL(
              'root-sw-js-static',
              '/root_sw.js',
            ) as string,
            { type: isDevEnv ? 'module' : 'classic' },
          )
          .then((registration) => {
            store.setRedirectSw(registration);
          });
      } else {
        store.setRedirectSw(null);
      }

      return () => destroy(store);
    },
    // `store` will never change. But list it as a dependency to make eslint
    // happy.
    [store],
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
            <RecoverableErrorBoundary>
              <BaseLayout />
            </RecoverableErrorBoundary>
          </AuthStateInitializer>
        </SyncedSearchParamsProvider>
      ),
      // Catch the errors that may happen in various providers.
      // Cannot use `<RecoverableErrorBoundary />` here because it requires the
      // auth state provider.
      errorElement: <RouteErrorDisplay />,
      children: [
        {
          path: 'login',
          element: (
            // We cannot use `errorElement` because it (react-router) doesn't
            // support error recovery.
            //
            // We handle the error at child level rather than at the parent
            // level because we want the error state to be reset when the user
            // navigates to a sibling view, which does not happen if the error
            // is handled by the parent (without additional logic).
            // The downside of this model is that we do not have a central place
            // for error handling, which is somewhat mitigated by applying the
            // same error boundary on all child routes.
            // The upside is that the error is naturally reset on route changes.
            //
            // A unique `key` is needed to ensure the boundary is not reused
            // when the user navigates to a sibling view. The error will be
            // naturally discarded as the route is unmounted.
            <RecoverableErrorBoundary key="login">
              <LoginPage />
            </RecoverableErrorBoundary>
          ),
        },
        { path: 'search', loader: searchRedirectionLoader },
        {
          path: 'builder-search',
          element: (
            <RecoverableErrorBoundary key="builder-search">
              <BuilderSearch />
            </RecoverableErrorBoundary>
          ),
        },
        {
          path: 'p/:project/test-search',
          element: (
            <RecoverableErrorBoundary key="test-search">
              <TestSearch />
            </RecoverableErrorBoundary>
          ),
        },
        {
          path: 'p/:project/builders',
          element: (
            <RecoverableErrorBoundary key="builders">
              <BuildersPage />
            </RecoverableErrorBoundary>
          ),
        },
        {
          path: 'p/:project/g/:group/builders',
          element: (
            <RecoverableErrorBoundary key="builder-groups">
              <BuildersPage />
            </RecoverableErrorBoundary>
          ),
        },
        {
          path: 'p/:project/builders/:bucket/:builder',
          element: (
            <RecoverableErrorBoundary key="builder">
              <BuilderPage />
            </RecoverableErrorBoundary>
          ),
        },
        {
          path: 'b/:buildId/*?',
          element: (
            <RecoverableErrorBoundary key="short-build">
              <BuildPageShortLink />
            </RecoverableErrorBoundary>
          ),
        },
        {
          path: 'p/:project/builders/:bucket/:builder/:buildNumOrId',
          element: (
            <RecoverableErrorBoundary key="long-build">
              <BuildPage />
            </RecoverableErrorBoundary>
          ),
          children: [
            {
              index: true,
              element: (
                <RecoverableErrorBoundary key="default">
                  <BuildDefaultTab />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: BuildPageTab.Overview,
              element: (
                <RecoverableErrorBoundary key="overview">
                  <OverviewTab />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: BuildPageTab.TestResults,
              element: (
                <RecoverableErrorBoundary key="test-results">
                  <TestResultsTab />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: BuildPageTab.Steps,
              element: (
                <RecoverableErrorBoundary key="steps">
                  <StepsTab />
                </RecoverableErrorBoundary>
              ),
              children: [
                // Some old systems generate links to a step by
                // appending suffix to /steps/ (crbug/1204954).
                // This allows those links to continue to work.
                { path: '*' },
              ],
            },
            {
              path: BuildPageTab.RelatedBuilds,
              element: (
                <RecoverableErrorBoundary key="related-builds">
                  <RelatedBuildsTab />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: BuildPageTab.Timeline,
              element: (
                <RecoverableErrorBoundary key="timeline">
                  <TimelineTab />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: BuildPageTab.Blamelist,
              element: (
                <RecoverableErrorBoundary key="blamelist">
                  <BlamelistTab />
                </RecoverableErrorBoundary>
              ),
            },
          ],
        },
        {
          path: 'inv/:invId',
          element: (
            <RecoverableErrorBoundary key="invocation">
              <InvocationPage />
            </RecoverableErrorBoundary>
          ),
          children: [
            {
              index: true,
              element: (
                <RecoverableErrorBoundary key="default">
                  <InvocationDefaultTab />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: 'test-results',
              element: (
                <RecoverableErrorBoundary key="test-results">
                  <TestResultsTab />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: 'invocation-details',
              element: (
                <RecoverableErrorBoundary key="invocation-details">
                  <InvocationDetailsTab />
                </RecoverableErrorBoundary>
              ),
            },
          ],
        },
        {
          path: 'artifact',
          element: (
            <RecoverableErrorBoundary key="artifact">
              <ArtifactPageLayout />
            </RecoverableErrorBoundary>
          ),
          children: [
            {
              path: 'text-diff/invocations/:invId/artifacts/:artifactId',
              element: (
                <RecoverableErrorBoundary key="test-text-diff">
                  <TextDiffArtifactPage />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: 'text-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
              element: (
                <RecoverableErrorBoundary key="result-text-diff">
                  <TextDiffArtifactPage />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: 'image-diff/invocations/:invId/artifacts/:artifactId',
              element: (
                <RecoverableErrorBoundary key="test-image-diff">
                  <ImageDiffArtifactPage />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: 'image-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
              element: (
                <RecoverableErrorBoundary key="result-image-diff">
                  <ImageDiffArtifactPage />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: 'raw/invocations/:invId/artifacts/:artifactId',
              element: (
                <RecoverableErrorBoundary key="test-raw">
                  <RawArtifactPage />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: 'raw/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
              element: (
                <RecoverableErrorBoundary key="result-raw">
                  <RawArtifactPage />
                </RecoverableErrorBoundary>
              ),
            },
          ],
        },
        {
          path: 'test/:projectOrRealm/:testId',
          element: (
            <RecoverableErrorBoundary key="test-history">
              <TestHistoryPage />
            </RecoverableErrorBoundary>
          ),
        },
        {
          path: '*',
          element: (
            <RecoverableErrorBoundary key="not-found">
              <NotFoundPage />
            </RecoverableErrorBoundary>
          ),
        },
        {
          path: 'bisection',
          element: (
            <RecoverableErrorBoundary key="bisection">
              <BisectionLayout />
            </RecoverableErrorBoundary>
          ),
          children: [
            {
              index: true,
              element: (
                <RecoverableErrorBoundary key="failure-analyses">
                  <FailureAnalysesPage />
                </RecoverableErrorBoundary>
              ),
            },
            {
              path: 'analysis/b/:bbid',
              element: (
                <RecoverableErrorBoundary key="analysis-details">
                  <AnalysisDetailsPage />
                </RecoverableErrorBoundary>
              ),
            },
          ],
        },
        {
          path: 'swarming',
          children: [
            {
              path: 'task/:taskId',
              element: (
                <RecoverableErrorBoundary key="swarming-task">
                  <SwarmingBuildPage />
                </RecoverableErrorBoundary>
              ),
            },
          ],
        },
      ],
    },
    {
      // We don't have a basename to stop react-router from handling non-SPA
      // routes (see the comments on the 'ui' route for rationale). We need to
      // to capture those routes and make the server handles it.
      path: '*',
      element: <ServerPage />,
      // Cannot use `<RecoverableErrorBoundary />` here because it requires the
      // auth state provider.
      errorElement: <RouteErrorDisplay />,
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
          <StoreProvider value={store}>
            <LitEnvProvider>
              <PageMetaProvider>
                <milo-tooltip />
                <RouterProvider router={router} />
              </PageMetaProvider>
            </LitEnvProvider>
          </StoreProvider>
        </QueryClientProvider>
      </ThemeProvider>
    </LocalizationProvider>
  );
}
