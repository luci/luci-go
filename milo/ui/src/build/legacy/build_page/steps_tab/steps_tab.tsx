// Copyright 2020 The LUCI Authors.
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

import { getBuildURLPath } from '@/common/tools/url_utils';
import { trackedRedirect } from '@/generic_libs/tools/react_router_utils';

/**
 * Redirects users to the infra tab.
 */
export function redirectToInfraTab({
  params,
  request,
}: LoaderFunctionArgs): Response {
  const { project, bucket, builder, buildNumOrId } = params;
  if (!project || !bucket || !builder || !buildNumOrId) {
    throw new Error(
      'invariant violated: project, bucket, builder, and buildNumOrId must be specified in the URL',
    );
  }

  const originalUrl = new URL(request.url);
  const buildUrl = getBuildURLPath({ project, bucket, builder }, buildNumOrId);
  return trackedRedirect({
    contentGroup: 'redirect | build | steps',
    // Track only origin + pathname to reduce the chance of including PII in the
    // URL.
    from: originalUrl.origin + originalUrl.pathname,
    to: `${buildUrl}/infra`,
  });
}

export const loader = redirectToInfraTab;
