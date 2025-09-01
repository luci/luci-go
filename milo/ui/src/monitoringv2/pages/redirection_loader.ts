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

import { LoaderFunctionArgs } from 'react-router';

import { trackedRedirect } from '@/generic_libs/tools/react_router_utils';

export const REDIRECT_TO_PROJECT = 'chromium';

// RedirectionLoader redirects /ui/labs/monitoringv2/* to /ui/monitoring/*.
export function redirectionLoader({ request }: LoaderFunctionArgs): Response {
  const url = new URL(request.url);
  const matches = url.pathname.match(/^\/ui\/labs\/monitoringv2(.*)$/);
  if (!matches || matches.length < 2) {
    throw new Error('invariant violated: url must match /ui/labs/monitoringv2');
  }
  return trackedRedirect({
    contentGroup: 'redirect | bisection',
    // Track only origin + pathname to reduce the chance of including PII in the
    // URL.
    from: url.origin + url.pathname,
    to: `/ui/monitoring${matches[1]}`,
  });
}

export const loader = redirectionLoader;
