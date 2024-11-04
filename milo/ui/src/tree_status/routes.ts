// Copyright 2024 The LUCI Authors.
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
export const treeStatusRoutes: RouteObject[] = [
  {
    path: ':tree',
    lazy: () => import('@/tree_status/pages/list_page'),
  },
];
