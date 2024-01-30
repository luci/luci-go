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

const DEFAULT_PAGE_SIZE = 50;

export function getPageSize(params: URLSearchParams) {
  return Number(params.get('limit')) || DEFAULT_PAGE_SIZE;
}

export function pageSizeUpdater(newPageSize: number) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (newPageSize === DEFAULT_PAGE_SIZE) {
      searchParams.delete('limit');
    } else {
      searchParams.set('limit', String(newPageSize));
    }
    return searchParams;
  };
}

export function getPageToken(params: URLSearchParams) {
  return params.get('cursor') || '';
}

export function pageTokenUpdater(newPageToken: string) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (!newPageToken) {
      searchParams.delete('cursor');
    } else {
      searchParams.set('cursor', newPageToken);
    }
    return searchParams;
  };
}
