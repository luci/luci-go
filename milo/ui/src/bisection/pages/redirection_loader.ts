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

import { LoaderFunctionArgs } from 'react-router-dom';

import { trackedRedirect } from '@/generic_libs/tools/react_router_utils';

export const REDIRECT_TO_PROJECT = 'chromium';

// RedirectionLoader redirects /ui/bisection/* to /ui/p/{PROJECT}/bisection/*.
// It also remap the sub path for compile bisection from `/analysis` to `/compile-analysis`.
// As we change the route for bisection pages, this redirection keeps old links works.
export function redirectionLoader({ request }: LoaderFunctionArgs): Response {
  const url = new URL(request.url);
  const matches = url.pathname.match(/^\/ui\/bisection(.*)$/);
  if (!matches || matches.length < 2) {
    throw new Error('invariant violated: url must match /ui/bisection');
  }
  const subPath = matches[1].replace(/^\/analysis/, '/compile-analysis');
  return trackedRedirect({
    contentGroup: 'redirect | bisection',
    // Track only origin + pathname to reduce the chance of including PII in the
    // URL.
    from: url.origin + url.pathname,
    to: `/ui/p/${REDIRECT_TO_PROJECT}/bisection${subPath}`,
  });
}

export const loader = redirectionLoader;
