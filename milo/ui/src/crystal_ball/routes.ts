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

import { createElement } from 'react';
import type { RouteObject } from 'react-router';

import { Layout } from '@/crystal_ball/components/layout/layout';

export const crystalBallRoutes: RouteObject[] = [
  {
    element: createElement(Layout),
    children: [
      {
        path: '',
        lazy: () => import('@/crystal_ball/pages/landing_page'),
      },
      {
        path: 'demo',
        lazy: () => import('@/crystal_ball/pages/demo_page/demo_page'),
      },
    ],
  },
];
