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

import { describe, expect, it, jest } from '@jest/globals';
import { renderHook, waitFor } from '@testing-library/react';

import { WrapperQueryOptions } from '@/common/types/query_wrapper_options';

import { useGapiQuery } from '../gapi_query';

import { useListBuilds } from './android_build';
import { ListBuildsResponse } from './types';

// Mock useGapiQuery
jest.mock('../gapi_query', () => ({
  useGapiQuery: jest.fn((_, options: WrapperQueryOptions<unknown>) => {
    return {
      data: options?.enabled === false ? undefined : { builds: [] },
      isLoading: false,
      isError: false,
    };
  }),
}));

describe('useListBuilds', () => {
  it('should call useGapiQuery with correct parameters', async () => {
    const { result } = renderHook(() =>
      useListBuilds(
        {
          branches: ['git_main'],
          page_size: 10,
        },
        {
          enabled: true,
        } as WrapperQueryOptions<ListBuildsResponse>,
      ),
    );

    await waitFor(() => expect(result.current.data).toBeDefined());

    expect(useGapiQuery).toHaveBeenCalledWith(
      expect.objectContaining({
        params: expect.objectContaining({
          branches: ['git_main'],
          page_size: 10,
          fields:
            'next_page_token,previous_page_token,builds(build_id,branch,target,creation_timestamp)',
        }),
      }),
      expect.objectContaining({ enabled: true }),
    );
  });

  it('should call useGapiQuery with custom fields when provided', async () => {
    const { result } = renderHook(() =>
      useListBuilds(
        {
          branches: ['git_main'],
          page_size: 10,
          fields: 'next_page_token,builds(build_id)',
        },
        {
          enabled: true,
        } as WrapperQueryOptions<ListBuildsResponse>,
      ),
    );

    await waitFor(() => expect(result.current.data).toBeDefined());

    expect(useGapiQuery).toHaveBeenCalledWith(
      expect.objectContaining({
        params: expect.objectContaining({
          branches: ['git_main'],
          page_size: 10,
          fields: 'next_page_token,builds(build_id)',
        }),
      }),
      expect.objectContaining({ enabled: true }),
    );
  });
});
