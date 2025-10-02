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

import { LoaderFunctionArgs, redirect } from 'react-router';

import { generateTestInvestigateUrlFromOld } from '@/common/tools/url_utils';

/**
 * This loader intercepts requests for the old test investigation URL,
 * reuses the 'generateTestInvestigateUrl' utility to construct the
 * new URL (preserving query params and hash), and then
 * returns a redirect response.
 */
export const legacyTestInvestigateLoader = ({
  request,
}: LoaderFunctionArgs) => {
  const url = new URL(request.url);

  // Reconstruct the original path, search, and hash
  const oldUrl = url.pathname + url.search + url.hash;

  // Use the utility we already built to convert it
  const newUrl = generateTestInvestigateUrlFromOld(oldUrl);

  // The route matched, so we always redirect.
  return redirect(newUrl);
};
