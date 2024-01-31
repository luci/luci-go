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

const DEFAULT_NUM_OF_BUILDS = 25;

export function getNumOfBuilds(params: URLSearchParams) {
  return Number(params.get('numOfBuilds')) || DEFAULT_NUM_OF_BUILDS;
}

export function numOfBuildsUpdater(newNumOfBuilds: number) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (newNumOfBuilds === DEFAULT_NUM_OF_BUILDS) {
      searchParams.delete('numOfBuilds');
    } else {
      searchParams.set('numOfBuilds', String(newNumOfBuilds));
    }
    return searchParams;
  };
}

export function getFilter(params: URLSearchParams) {
  return params.get('q') || '';
}

export function filterUpdater(newSearchQuery: string) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (newSearchQuery === '') {
      searchParams.delete('q');
    } else {
      searchParams.set('q', newSearchQuery);
    }
    return searchParams;
  };
}
