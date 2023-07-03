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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { ThemeProvider } from '@emotion/react';
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
import { BaseLayout as BisectionBaseLayout } from '@/bisection/layouts/base';
import { AnalysisDetailsPage } from '@/bisection/pages/analysis_details';
import { FailureAnalysesPage } from '@/bisection/pages/failure_analyses';
import { obtainAuthState } from '@/common/api/auth_state';
import { LitEnvProvider } from '@/common/components/lit_env_provider';
import { BaseLayout } from '@/common/layouts/base';
import { Store, StoreProvider } from '@/common/store';
import { theme } from '@/common/themes/base';
import { createStaticTrustedURL } from '@/generic_libs/tools/utils';

import { ArtifactPageLayout } from './pages/artifact/artifact_page_layout';
import { ImageDiffArtifactPage } from './pages/artifact/image_diff_artifact_page';
import { RawArtifactPage } from './pages/artifact/raw_artifact_page';
import { TextDiffArtifactPage } from './pages/artifact/text_diff_artifact_page';
import { BuildPage, BuildPageShortLink } from './pages/build_page';
import { BlamelistTab } from './pages/build_page/blamelist_tab';
import { BuildDefaultTab } from './pages/build_page/build_default_tab';
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
import { SearchPage } from './pages/search_page/search_page';
import { TestHistoryPage } from './pages/test_history_page/test_history_page';
import { TestResultsTab } from './pages/test_results_tab/test_results_tab';

const QUERY_CLIENT_CONFIG: QueryClientConfig = {
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => {
        // Do not retry when the errors is non transient.
        if (
          error instanceof GrpcError &&
          [
            RpcCode.INVALID_ARGUMENT,
            RpcCode.PERMISSION_DENIED,
            RpcCode.UNAUTHENTICATED,
            RpcCode.UNIMPLEMENTED,
          ].includes(error.code)
        ) {
          return false;
        }

        // Keep the default retry behavior otherwise.
        return failureCount < 3;
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
    Store.create({}, { isDevEnv: firstInitOpts.current.isDevEnv })
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
          { type: isDevEnv ? 'module' : 'classic' }
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
              '/root_sw.js'
            ) as string,
            { type: isDevEnv ? 'module' : 'classic' }
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
    [store]
  );

  const router = createBrowserRouter([
    {
      // Use a 'ui' route to enclose all routes instead of setting the basename
      // to 'ui' so the URLs work the same whether they are consumed by a
      // component/function imported from 'react-router' or from other modules.
      path: 'ui',
      element: <BaseLayout />,
      loader: async () => obtainAuthState(),
      children: [
        { path: 'login', element: <LoginPage /> },
        { path: 'search', element: <SearchPage /> },
        { path: 'p/:project/builders', element: <BuildersPage /> },
        { path: 'p/:project/g/:group/builders', element: <BuildersPage /> },
        {
          path: 'p/:project/builders/:bucket/:builder',
          element: <BuilderPage />,
        },
        { path: 'b/:buildId/*?', element: <BuildPageShortLink /> },
        {
          path: 'p/:project/builders/:bucket/:builder/:buildNumOrId',
          element: <BuildPage />,
          children: [
            { index: true, element: <BuildDefaultTab /> },
            { path: 'overview', element: <OverviewTab /> },
            { path: 'test-results', element: <TestResultsTab /> },
            {
              path: 'steps',
              element: <StepsTab />,
              children: [
                // Some old systems generate links to a step by appending suffix
                // to /steps/ (crbug/1204954).
                // This allows those links to continue to work.
                { path: '*' },
              ],
            },
            { path: 'related-builds', element: <RelatedBuildsTab /> },
            { path: 'timeline', element: <TimelineTab /> },
            { path: 'blamelist', element: <BlamelistTab /> },
          ],
        },
        {
          path: 'inv/:invId',
          element: <InvocationPage />,
          children: [
            { index: true, element: <InvocationDefaultTab /> },
            { path: 'test-results', element: <TestResultsTab /> },
            { path: 'invocation-details', element: <InvocationDetailsTab /> },
          ],
        },
        {
          path: 'artifact',
          element: <ArtifactPageLayout />,
          children: [
            {
              path: 'text-diff/invocations/:invId/artifacts/:artifactId',
              element: <TextDiffArtifactPage />,
            },
            {
              path: 'text-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
              element: <TextDiffArtifactPage />,
            },
            {
              path: 'image-diff/invocations/:invId/artifacts/:artifactId',
              element: <ImageDiffArtifactPage />,
            },
            {
              path: 'image-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
              element: <ImageDiffArtifactPage />,
            },
            {
              path: 'raw/invocations/:invId/artifacts/:artifactId',
              element: <RawArtifactPage />,
            },
            {
              path: 'raw/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
              element: <RawArtifactPage />,
            },
          ],
        },
        { path: 'test/:projectOrRealm/:testId', element: <TestHistoryPage /> },
        { path: '*', element: <NotFoundPage /> },
        {
          path: 'bisection',
          element: <BisectionBaseLayout />,
          children: [
            {
              index: true,
              element: <FailureAnalysesPage />,
            },
            {
              path: 'analysis/b/:bbid',
              element: <AnalysisDetailsPage />,
            },
          ],
        },
      ],
    },
  ]);

  return (
    <StoreProvider value={store}>
      <ThemeProvider theme={theme}>
        <QueryClientProvider client={queryClient}>
          <LitEnvProvider>
            <milo-tooltip />
            <RouterProvider router={router} />
          </LitEnvProvider>
        </QueryClientProvider>
      </ThemeProvider>
    </StoreProvider>
  );
}
