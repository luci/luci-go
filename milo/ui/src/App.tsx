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

// By default the file will be loaded as a constructive stylesheet by
// lit-css-loader. This doesn't work for React, so we need to tell webpack the
// exact loaders to use.
import '!!style-loader!css-loader?modules=global!./styles/common_style.css';
import { destroy } from 'mobx-state-tree';
import { useEffect, useState } from 'react';
import { Route, Routes } from 'react-router-dom';

import './components/tooltip';
import { LitEnvProvider } from './components/lit_env_provider';
import { BaseLayout } from './layouts/base';
import { REFRESH_AUTH_CHANNEL } from './libs/constants';
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
import { BuildersPage } from './pages/builders_page';
import { AuthChannelClosePage } from './pages/close_page';
import { LoginPage } from './pages/login_page';
import { NotFoundPage } from './pages/not_found_page';
import { SearchPage } from './pages/search_page';
import { TestResultsTab } from './pages/test_results_tab';
import { Store, StoreProvider } from './store';

export function App() {
  const [store] = useState(() => Store.create());

  useEffect(() => {
    store.authState.init();

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

    const onRefreshAuth = () => store.authState.scheduleUpdate(true);
    REFRESH_AUTH_CHANNEL.addEventListener('message', onRefreshAuth);

    return () => {
      REFRESH_AUTH_CHANNEL.removeEventListener('message', onRefreshAuth);
      destroy(store);
    };
  }, []);

  return (
    <StoreProvider value={store}>
      <LitEnvProvider>
        <milo-tooltip />
        <Routes>
          <Route path="/ui" element={<BaseLayout />}>
            <Route path="login" element={<LoginPage />} />
            <Route path="search" element={<SearchPage />} />
            <Route path="auth-callback/:channelId" element={<AuthChannelClosePage />} />
            <Route path="p/:project/builders" element={<BuildersPage />} />
            <Route path="p/:project/g/:group/builders" element={<BuildersPage />} />
            <Route path="b/:buildId/*?" element={<BuildPageShortLink />} />
            <Route path="p/:project/builders/:bucket/:builder/:buildNumOrId" element={<BuildPage />}>
              <Route index element={<BuildDefaultTab />} />
              <Route path="overview" element={<OverviewTab />} />
              <Route path="test-results" element={<TestResultsTab />} />
              <Route path="steps" element={<StepsTab />}>
                {/* Some old systems generate links to a step by appending
                suffix to /steps/ (crbug/1204954).
                This allows those links to continue to work. */}
                <Route path="*" />
              </Route>
              <Route path="related-builds" element={<RelatedBuildsTab />} />
              <Route path="timeline" element={<TimelineTab />} />
              <Route path="blamelist" element={<BlamelistTab />} />
            </Route>
            <Route path="artifact" element={<ArtifactPageLayout />}>
              <Route
                path="text-diff/invocations/:invId/tests/:testId/artifacts/:artifactId"
                element={<TextDiffArtifactPage />}
              />
              <Route
                path="text-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId"
                element={<TextDiffArtifactPage />}
              />
              <Route
                path="image-diff/invocations/:invId/tests/:testId/artifacts/:artifactId"
                element={<ImageDiffArtifactPage />}
              />
              <Route
                path="image-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId"
                element={<ImageDiffArtifactPage />}
              />
              <Route path="raw/invocations/:invId/tests/:testId/artifacts/:artifactId" element={<RawArtifactPage />} />
              <Route
                path="raw/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId"
                element={<RawArtifactPage />}
              />
            </Route>
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Routes>
      </LitEnvProvider>
    </StoreProvider>
  );
}
