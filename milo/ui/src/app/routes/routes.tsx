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

import type { RouteObject } from 'react-router-dom';

export const routes: RouteObject[] = [
  {
    index: true,
    lazy: () => import('@/app/pages/search/project_search'),
  },
  {
    path: 'login',
    lazy: () => import('@/app/pages/login_page'),
  },
  {
    path: 'search',
    lazy: () => import('@/app/pages/search/search_redirection_loader'),
  },
  {
    path: 'builder-search',
    lazy: () => import('@/build/pages/builder_search_page'),
  },
  {
    path: 'p/:project/test-search',
    lazy: () => import('@/app/pages/search/test_search'),
  },
  {
    path: 'p/:project',
    lazy: () => import('@/build/pages/console_list_page'),
  },
  {
    path: 'p/:project/builders',
    lazy: () => import('@/app/pages/builders_page'),
  },
  {
    path: 'p/:project/g/:group/builders',
    lazy: () => import('@/app/pages/builders_page'),
  },
  {
    path: 'p/:project/builders/:bucket/:builder',
    lazy: () => import('@/build/pages/builder_page'),
  },
  {
    path: 'b/:buildId/*?',
    lazy: () => import('@/app/pages/build_page/build_page_short_link'),
  },
  {
    path: 'p/:project/builders/:bucket/:builder/:buildNumOrId',
    lazy: () => import('@/app/pages/build_page'),
    children: [
      {
        index: true,
        lazy: () => import('@/app/pages/build_page/build_default_tab'),
      },
      {
        path: 'overview',
        lazy: () => import('@/app/pages/build_page/overview_tab'),
      },
      {
        path: 'test-results',
        lazy: () => import('@/app/pages/test_results_tab'),
      },
      {
        path: 'steps',
        lazy: () => import('@/app/pages/build_page/steps_tab'),
        children: [
          // Some old systems generate links to a step by
          // appending suffix to /steps/ (crbug/1204954).
          // This allows those links to continue to work.
          { path: '*' },
        ],
      },
      {
        path: 'related-builds',
        lazy: () => import('@/app/pages/build_page/related_builds_tab'),
      },
      {
        path: 'timeline',
        lazy: () => import('@/app/pages/build_page/timeline_tab'),
      },
      {
        path: 'blamelist',
        lazy: () => import('@/app/pages/build_page/blamelist_tab'),
      },
    ],
  },
  {
    path: 'inv/:invId',
    lazy: () => import('@/app/pages/invocation_page'),
    children: [
      {
        index: true,
        lazy: () =>
          import('@/app/pages/invocation_page/invocation_default_tab'),
      },
      {
        path: 'test-results',
        lazy: () => import('@/app/pages/test_results_tab'),
      },
      {
        path: 'invocation-details',
        lazy: () =>
          import('@/app/pages/invocation_page/invocation_details_tab'),
      },
    ],
  },
  {
    path: 'artifact',
    lazy: () => import('@/app/pages/artifact/artifact_page_layout'),
    children: [
      {
        path: 'text-diff/invocations/:invId/artifacts/:artifactId',
        lazy: () => import('@/app/pages/artifact/text_diff_artifact_page'),
      },
      {
        path: 'text-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
        lazy: () => import('@/app/pages/artifact/text_diff_artifact_page'),
      },
      {
        path: 'image-diff/invocations/:invId/artifacts/:artifactId',
        lazy: () => import('@/app/pages/artifact/image_diff_artifact_page'),
      },
      {
        path: 'image-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
        lazy: () => import('@/app/pages/artifact/image_diff_artifact_page'),
      },
      {
        path: 'raw/invocations/:invId/artifacts/:artifactId',
        lazy: () => import('@/app/pages/artifact/raw_artifact_page'),
      },
      {
        path: 'raw/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
        lazy: () => import('@/app/pages/artifact/raw_artifact_page'),
      },
    ],
  },
  {
    path: 'test/:projectOrRealm/:testId',
    lazy: () => import('@/app/pages/test_history_page'),
  },
  {
    path: 'bisection/*',
    lazy: () => import('@/bisection/pages/redirection_loader'),
  },
  {
    path: 'p/:project/bisection',
    lazy: () => import('@/bisection/layouts/base'),
    children: [
      {
        path: '',
        lazy: () => import('@/bisection/pages/analyses_page'),
        children: [
          {
            index: true,
            lazy: () => import('@/bisection/pages/analyses_default_tab'),
          },
          {
            path: 'compile-analysis',
            lazy: () => import('@/bisection/pages/compile_analyses_tab'),
          },
          {
            path: 'test-analysis',
            lazy: () => import('@/bisection/pages/test_analyses_tab'),
          },
        ],
      },
      {
        path: 'compile-analysis/b/:bbid',
        lazy: () => import('@/bisection/pages/analysis_details'),
      },
      {
        path: 'test-analysis/b/:id',
        lazy: () => import('@/bisection/pages/test_analysis_details'),
      },
    ],
  },
  {
    path: 'swarming',
    children: [
      {
        path: 'task/:taskId',
        lazy: () => import('@/swarming/views/swarming_build_page'),
      },
    ],
  },
  {
    path: 'doc/release-notes',
    lazy: () => import('@/app/pages/release_notes_page'),
  },
  {
    path: 'labs',
    children: [
      {
        // TODO(b/308856913): Fix all outstanding todo's before promoting the page to production.
        path: 'p/:project/inv/:invID/test/:testID/variant/:vHash',
        lazy: () => import('@/test_verdict/pages/test_verdict_page'),
      },
      {
        path: 'monitoring',
        lazy: () => import('@/monitoring/pages/monitoring_page'),
      },
      {
        path: 'monitoring/:tree',
        lazy: () => import('@/monitoring/pages/monitoring_page'),
      },
      {
        path: 'p/:project/builders',
        lazy: () => import('@/build/pages/builder_list_page'),
      },
      {
        path: 'tree-status/:tree',
        lazy: () => import('@/tree_status/pages/list_page'),
      },
    ],
  },
];
