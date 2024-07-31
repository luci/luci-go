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

export function getPageSize(params: URLSearchParams, defaultPageSize: number) {
  return Number(params.get('limit')) || defaultPageSize;
}

export function pageSizeUpdater(newPageSize: number, defaultPageSize: number) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (newPageSize === defaultPageSize) {
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
