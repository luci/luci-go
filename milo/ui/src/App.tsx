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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { destroy } from 'mobx-state-tree';
import { useEffect, useState } from 'react';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';

import './styles/common_style.css';
import './styles/color_classes.css';
import './components/tooltip';
import { LitEnvProvider } from './components/lit_env_provider';
import { BaseLayout } from './layouts/base';
import { obtainAuthState } from './libs/auth_state';
import { createStaticTrustedURL } from './libs/utils';
import { ArtifactPageLayout } from './pages/artifact/artifact_page_layout';
import { ImageDiffArtifactPage } from './pages/artifact/image_diff_artifact_page';
import { RawArtifactPage } from './pages/artifact/raw_artifact_page';
import { TextDiffArtifactPage } from './pages/artifact/text_diff_artifact_page';
import { BuildPage, BuildPageShortLink } from './pages/build_page';
import { BlamelistTab } from './pages/build_page/blamelist_tab';
import { BuildDefaultTab } from './pages/build_page/build_default_tab';
import { OverviewTab } from './pages/build_page/overview_tab';
import { RelatedBuildsTab } from './pages/build_page/related_builds_tab';
import { StepsTab } from './pages/build_page/steps_tab';
import { TimelineTab } from './pages/build_page/timeline_tab';
import { BuilderPage } from './pages/builder_page';
import { BuildersPage } from './pages/builders_page';
import { InvocationPage } from './pages/invocation_page';
import { InvocationDefaultTab } from './pages/invocation_page/invocation_default_tab';
import { InvocationDetailsTab } from './pages/invocation_page/invocation_details_tab';
import { LoginPage } from './pages/login_page';
import { NotFoundPage } from './pages/not_found_page';
import { SearchPage } from './pages/search_page';
import { TestHistoryPage } from './pages/test_history_page';
import { TestResultsTab } from './pages/test_results_tab';
import { Store, StoreProvider } from './store';
import { theme } from './theme';

export function App() {
  const [store] = useState(() => Store.create());
  const [queryClient] = useState(() => new QueryClient());

  useEffect(() => {
    // Expose `store` in the global namespace to make inspecting/debugging the
    // store via the browser dev-tool easier.
    //
    // The __STORE variable should only be used for debugging purpose. As such,
    // do not declare __STORE as a global variable explicity so it's less likely
    // to be misused.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (window as any).__STORE = store;

    if (navigator.serviceWorker && ENABLE_UI_SW) {
      store.workbox.init(createStaticTrustedURL('sw-js-static', '/ui/service-worker.js'));
    }
    if (navigator.serviceWorker && !document.cookie.includes('showNewBuildPage=false')) {
      navigator.serviceWorker
        .register(
          // cast to string because TypeScript doesn't allow us to use
          // TrustedScriptURL here
          createStaticTrustedURL('root-sw-js-static', '/root-sw.js') as string
        )
        .then((registration) => {
          store.setRedirectSw(registration);
        });
    } else {
      store.setRedirectSw(null);
    }

    return () => destroy(store);
  }, []);

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
            { path: 'text-diff/invocations/:invId/artifacts/:artifactId', element: <TextDiffArtifactPage /> },
            {
              path: 'text-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
              element: <TextDiffArtifactPage />,
            },
            { path: 'image-diff/invocations/:invId/artifacts/:artifactId', element: <ImageDiffArtifactPage /> },
            {
              path: 'image-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
              element: <ImageDiffArtifactPage />,
            },
            { path: 'raw/invocations/:invId/artifacts/:artifactId', element: <RawArtifactPage /> },
            {
              path: 'raw/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
              element: <RawArtifactPage />,
            },
          ],
        },
        { path: 'test/:realm/:testId', element: <TestHistoryPage /> },
        { path: '*', element: <NotFoundPage /> },
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
