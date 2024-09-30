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

// IMPORTANT:
// When adding new routes, ensure that the path param does not contain PII.
// If you need PII in the path param, document it and scrub the URL param from
// GA4 tracking. See http://go/ooga-config#scrub-urls.
export const routes: RouteObject[] = [
  {
    index: true,
    lazy: () => import('@/core/pages/project_search_page'),
  },
  {
    path: 'login',
    lazy: () => import('@/core/pages/login_page'),
  },
  {
    path: 'local-login-instruction',
    lazy: () => import('@/core/pages/local_login_instruction_page'),
  },
  {
    path: 'search',
    lazy: () => import('@/routes/search_loader/search_redirection_loader'),
  },
  {
    path: 'builder-search',
    lazy: () => import('@/build/pages/builder_search_page'),
  },
  {
    path: 'p/:project/test-search',
    lazy: () => import('@/test_verdict/pages/test_search'),
  },
  {
    path: 'p/:project',
    lazy: () => import('@/build/pages/console_list_page'),
  },
  {
    path: 'p/:project/builders',
    lazy: () => import('@/build/pages/builder_list_page'),
  },
  {
    path: 'p/:project/g/:group/builders',
    lazy: () => import('@/build/pages/builder_group_page'),
  },
  {
    path: 'p/:project/builders/:bucket/:builder',
    lazy: () => import('@/build/pages/builder_page'),
  },
  {
    path: 'b/:buildId/*?',
    lazy: () => import('@/build/legacy/build_page/build_page_short_link'),
  },
  {
    path: 'p/:project/builders/:bucket/:builder/:buildNumOrId',
    lazy: () => import('@/build/legacy/build_page'),
    children: [
      {
        index: true,
        lazy: () => import('@/build/legacy/build_page/build_default_tab'),
      },
      {
        path: 'overview',
        lazy: () => import('@/build/legacy/build_page/overview_tab'),
      },
      {
        path: 'infra',
        lazy: () => import('@/build/legacy/build_page/infra_tab'),
      },
      {
        path: 'test-results',
        lazy: () => import('@/test_verdict/legacy/test_results_tab'),
      },
      {
        path: 'steps',
        lazy: () => import('@/build/legacy/build_page/steps_tab'),
        children: [
          // Some old systems generate links to a step by
          // appending suffix to /steps/ (crbug/1204954).
          // This allows those links to continue to work.
          { path: '*' },
        ],
      },
      {
        path: 'related-builds',
        lazy: () => import('@/build/legacy/build_page/related_builds_tab'),
      },
      {
        path: 'timeline',
        lazy: () => import('@/build/legacy/build_page/timeline_tab'),
      },
      {
        path: 'blamelist',
        lazy: () => import('@/build/legacy/build_page/blamelist_tab'),
      },
    ],
  },
  {
    path: 'inv/:invId',
    lazy: () => import('@/test_verdict/legacy/invocation_page'),
    children: [
      {
        index: true,
        lazy: () =>
          import(
            '@/test_verdict/legacy/invocation_page/invocation_default_tab'
          ),
      },
      {
        path: 'test-results',
        lazy: () => import('@/test_verdict/legacy/test_results_tab'),
      },
      {
        path: 'invocation-details',
        lazy: () => import('@/test_verdict/pages/invocation_page/details_tab'),
      },
    ],
  },
  {
    path: 'artifact',
    lazy: () => import('@/test_verdict/legacy/artifact/artifact_page_layout'),
    children: [
      {
        path: 'text-diff/invocations/:invId/artifacts/:artifactId',
        lazy: () =>
          import('@/test_verdict/legacy/artifact/text_diff_artifact_page'),
      },
      {
        path: 'text-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
        lazy: () =>
          import('@/test_verdict/legacy/artifact/text_diff_artifact_page'),
      },
      {
        path: 'image-diff/invocations/:invId/artifacts/:artifactId',
        lazy: () =>
          import('@/test_verdict/legacy/artifact/image_diff_artifact_page'),
      },
      {
        path: 'image-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
        lazy: () =>
          import('@/test_verdict/legacy/artifact/image_diff_artifact_page'),
      },
      {
        path: 'raw/invocations/:invId/artifacts/:artifactId',
        lazy: () => import('@/test_verdict/legacy/artifact/raw_artifact_page'),
      },
      {
        path: 'raw/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
        lazy: () => import('@/test_verdict/legacy/artifact/raw_artifact_page'),
      },
    ],
  },
  {
    path: 'test/:projectOrRealm/:testId',
    lazy: () => import('@/test_verdict/legacy/test_history_page'),
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
        lazy: () => import('@/swarming/pages/swarming_build_page'),
      },
    ],
  },
  {
    path: 'doc/release-notes',
    lazy: () => import('@/core/pages/release_notes_page'),
  },
  {
    path: 'tree-status/:tree',
    lazy: () => import('@/tree_status/pages/list_page'),
  },
  {
    path: 'labs',
    lazy: () => import('@/core/pages/labs_page'),
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
        path: 'monitoringv2',
        lazy: () => import('@/monitoringv2/pages/monitoring_page'),
      },
      {
        path: 'monitoringv2/:tree',
        lazy: () => import('@/monitoringv2/pages/monitoring_page'),
      },
      {
        // TODO: Remove this endpoint after we remove all client.
        path: 'tree-status/:tree',
        lazy: () => import('@/tree_status/pages/redirection_loader'),
      },
      {
        path: 'p/:project/regressions',
        lazy: () => import('@/test_verdict/pages/recent_regressions_page'),
      },
      {
        // TODO(b/321110247): once we have a stable regression group ID, replace
        // "details" with the regression group ID.
        path: 'p/:project/regressions/details',
        lazy: () => import('@/test_verdict/pages/regression_details_page'),
      },
      {
        path: 'p/:project/tests/:testId/variants/:variantHash/refs/:refHash/blamelist',
        lazy: () => import('@/test_verdict/pages/blamelist_page'),
      },
      {
        path: 'p/:project/log-search',
        lazy: () => import('@/test_verdict/pages/log_search_page'),
        children: [
          {
            index: true,
            lazy: () =>
              import('@/test_verdict/pages/log_search_page/log_default_tab'),
          },
          {
            path: 'test-logs',
            lazy: () =>
              import('@/test_verdict/pages/log_search_page/test_logs_tab'),
          },
          {
            path: 'shared-logs',
            lazy: () =>
              import('@/test_verdict/pages/log_search_page/shared_logs_tab'),
          },
        ],
      },
      {
        path: 'inv/:invId',
        lazy: () => import('@/test_verdict/pages/invocation_page'),
        children: [
          {
            path: 'invocation-details',
            lazy: () =>
              import('@/test_verdict/pages/invocation_page/details_tab'),
          },
        ],
      },
      {
        path: 'auth/groups/*',
        lazy: () => import('@/authdb/pages/groups_page'),
      },
    ],
  },
];
