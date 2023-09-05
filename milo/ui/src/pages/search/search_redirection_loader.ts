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

import { redirect } from 'react-router-dom';

export enum SearchTarget {
  Builders = 'BUILDERS',
  Tests = 'TESTS',
}

export const DEFAULT_SEARCH_TARGET = SearchTarget.Builders;
export const DEFAULT_TEST_PROJECT = 'chromium';

export const searchRedirectionLoader = ({
  request,
}: {
  request: Request;
}): Response => {
  const url = new URL(request.url);
  const searchParams = url.searchParams;

  const testProject = searchParams.get('tp') || DEFAULT_TEST_PROJECT;

  const searchQuery = searchParams.get('q') || '';
  const t = searchParams.get('t');
  const searchTarget = Object.values(SearchTarget).includes(t as SearchTarget)
    ? (t as SearchTarget)
    : DEFAULT_SEARCH_TARGET;

  if (searchTarget === SearchTarget.Builders) {
    return redirect(`/ui/builder-search?q=${searchQuery}`);
  } else {
    return redirect(`/ui/p/${testProject}/test-search?q=${searchQuery}`);
  }
};

export const loader = searchRedirectionLoader;
