// Copyright 2025 The LUCI Authors.
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

import { RouteObject } from 'react-router';

import { legacyTestInvestigateLoader } from '@/test_investigation/loaders/legacy_redirect_loader';

export const testInvestigationRoutes: RouteObject[] = [
  // This route catches the OLD test page URL
  {
    path: 'invocations/:invocationId/tests/:testId/variants/:variantHash',
    loader: legacyTestInvestigateLoader,
    element: null,
  },

  {
    path: 'invocations/:invocationId',
    lazy: () => import('@/test_investigation/pages/invocation_page'),
    children: [
      {
        index: true,
        loader: async () => {
          const { redirect } = await import('react-router');
          return redirect('tests');
        },
      },
      {
        path: 'tests',
        lazy: async () => {
          const { TestTab } = await import(
            '@/test_investigation/components/invocation_page/tabs'
          );
          return { Component: TestTab };
        },
      },
      {
        path: 'summary',
        lazy: async () => {
          const { SummaryTab } = await import(
            '@/test_investigation/components/invocation_page/tabs'
          );
          return { Component: SummaryTab };
        },
      },
      {
        path: 'properties',
        lazy: async () => {
          const { PropertiesTab } = await import(
            '@/test_investigation/components/invocation_page/tabs'
          );
          return { Component: PropertiesTab };
        },
      },
    ],
  },

  {
    path: 'invocations/:invocationId/modules/:module/schemes/:scheme/variants/:variant/coarse/:coarse/fine/:fine/cases/:case',
    lazy: () => import('@/test_investigation/pages/test_investigate_page'),
  },
  {
    path: 'invocations/:invocationId/modules/:module/schemes/:scheme/variants/:variant/coarse/:coarse/cases/:case',
    lazy: () => import('@/test_investigation/pages/test_investigate_page'),
  },
  {
    path: 'invocations/:invocationId/modules/:module/schemes/:scheme/variants/:variant/fine/:fine/cases/:case',
    lazy: () => import('@/test_investigation/pages/test_investigate_page'),
  },
  {
    path: 'invocations/:invocationId/modules/:module/schemes/:scheme/variants/:variant/cases/:case',
    lazy: () => import('@/test_investigation/pages/test_investigate_page'),
  },
];
