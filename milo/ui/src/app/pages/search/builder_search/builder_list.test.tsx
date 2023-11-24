// Copyright 2022 The LUCI Authors.
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

import { act, cleanup, render } from '@testing-library/react';

import { BuilderListDisplay } from '@/app/pages/search/builder_search/builder_list_display';
import { useInfinitePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import {
  ListBuildersResponse,
  MiloInternal,
} from '@/common/services/milo_internal';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuilderList } from './builder_list';

jest.mock('@/common/hooks/legacy_prpc_query', () => {
  return createSelectiveSpiesFromModule<
    typeof import('@/common/hooks/legacy_prpc_query')
  >('@/common/hooks/legacy_prpc_query', ['useInfinitePrpcQuery']);
});

jest.mock('@/app/pages/search/builder_search/builder_list_display', () => {
  return createSelectiveSpiesFromModule<
    typeof import('@/app/pages/search/builder_search/builder_list_display')
  >('@/app/pages/search/builder_search/builder_list_display', [
    'BuilderListDisplay',
  ]);
});

const builderPages: { [pageToken: string]: ListBuildersResponse } = {
  '': {
    builders: [
      {
        id: { project: 'proj1', bucket: 'bucket1', builder: 'builder1' },
        config: {},
      },
      {
        id: { project: 'proj1', bucket: 'bucket1', builder: 'builder2' },
        config: {},
      },
    ],
    nextPageToken: 'page2',
  },
  page2: {
    builders: [
      {
        id: { project: 'proj1', bucket: 'bucket2', builder: 'builder1' },
        config: {},
      },
      {
        id: { project: 'proj1', bucket: 'bucket2', builder: 'builder2' },
        config: {},
      },
    ],
    nextPageToken: 'page3',
  },
  page3: {
    builders: [
      {
        id: { project: 'proj2', bucket: 'bucket1', builder: 'builder1' },
        config: {},
      },
      {
        id: { project: 'proj2', bucket: 'bucket1', builder: 'builder2' },
        config: {},
      },
    ],
  },
};

describe('BuilderList', () => {
  let useInfinitePrpcQuerySpy: jest.MockedFunction<typeof useInfinitePrpcQuery>;
  let listBuilderMock: jest.SpyInstance;
  let builderListDisplaySpy: jest.MockedFunctionDeep<typeof BuilderListDisplay>;

  beforeEach(() => {
    jest.useFakeTimers();
    useInfinitePrpcQuerySpy = jest.mocked(useInfinitePrpcQuery);
    listBuilderMock = jest
      .spyOn(MiloInternal.prototype, 'listBuilders')
      .mockImplementation(async ({ pageToken }) => {
        return builderPages[pageToken || ''];
      });
    builderListDisplaySpy = jest.mocked(BuilderListDisplay);
  });
  afterEach(() => {
    cleanup();
    jest.useRealTimers();
  });

  it('e2e', async () => {
    const { rerender } = render(
      <FakeContextProvider>
        <BuilderList searchQuery="" />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());
    await act(() => jest.runAllTimersAsync());
    await act(() => jest.runAllTimersAsync());

    expect(useInfinitePrpcQuerySpy).toHaveBeenCalledWith({
      host: '',
      insecure: location.protocol === 'http:',
      Service: MiloInternal,
      method: 'listBuilders',
      request: { pageSize: expect.anything() },
    });
    // All the pages should've been be loaded.
    expect(listBuilderMock).toHaveBeenCalledTimes(3);
    expect(builderListDisplaySpy).toHaveBeenLastCalledWith(
      {
        groupedBuilders: {
          'proj1/bucket1': [
            { project: 'proj1', bucket: 'bucket1', builder: 'builder1' },
            { project: 'proj1', bucket: 'bucket1', builder: 'builder2' },
          ],
          'proj1/bucket2': [
            { project: 'proj1', bucket: 'bucket2', builder: 'builder1' },
            { project: 'proj1', bucket: 'bucket2', builder: 'builder2' },
          ],
          'proj2/bucket1': [
            { project: 'proj2', bucket: 'bucket1', builder: 'builder1' },
            { project: 'proj2', bucket: 'bucket1', builder: 'builder2' },
          ],
        },
        isLoading: false,
      },
      expect.anything(),
    );

    // Filter builder.
    rerender(
      <FakeContextProvider>
        <BuilderList searchQuery="builder2" />
      </FakeContextProvider>,
    );
    // Do not trigger more list builder calls.
    expect(listBuilderMock).toHaveBeenCalledTimes(3);
    // Builders are filtered correctly.
    expect(builderListDisplaySpy).toHaveBeenLastCalledWith(
      {
        groupedBuilders: {
          'proj1/bucket1': [
            { project: 'proj1', bucket: 'bucket1', builder: 'builder2' },
          ],
          'proj1/bucket2': [
            { project: 'proj1', bucket: 'bucket2', builder: 'builder2' },
          ],
          'proj2/bucket1': [
            { project: 'proj2', bucket: 'bucket1', builder: 'builder2' },
          ],
        },
        isLoading: false,
      },
      expect.anything(),
    );

    // Fuzzy search builder.
    rerender(
      <FakeContextProvider>
        <BuilderList searchQuery="CkEt1/bU Oj2" />
      </FakeContextProvider>,
    );
    // Do not trigger more list builder calls.
    expect(listBuilderMock).toHaveBeenCalledTimes(3);
    // Builders are filtered correctly.
    expect(builderListDisplaySpy).toHaveBeenLastCalledWith(
      {
        groupedBuilders: {
          'proj2/bucket1': [
            { project: 'proj2', bucket: 'bucket1', builder: 'builder1' },
            { project: 'proj2', bucket: 'bucket1', builder: 'builder2' },
          ],
        },
        isLoading: false,
      },
      expect.anything(),
    );
  });
});
