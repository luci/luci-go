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

import { render } from '@testing-library/react';
import { act } from 'react';

import { FilterableBuilderTable } from '@/build/components/filterable_builder_table';
import {
  BuilderID,
  BuilderItem,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import {
  ListBuildersResponse,
  MiloInternalClientImpl,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuilderGroupPage } from './builder_group_page';

jest.mock('@/build/components/filterable_builder_table', () =>
  self.createSelectiveMockFromModule<
    typeof import('@/build/components/filterable_builder_table')
  >('@/build/components/filterable_builder_table', ['FilterableBuilderTable']),
);

const builders = Array(25)
  .fill(0)
  .map((_, i) =>
    BuilderID.fromPartial({
      project: 'proj',
      bucket: `bucket${i}`,
      builder: `builder${i}`,
    }),
  );

const builderItems = builders.map((id) => BuilderItem.fromPartial({ id }));

const pages: { [key: string]: ListBuildersResponse } = {
  '': ListBuildersResponse.fromPartial({
    builders: builderItems.slice(0, 10),
    nextPageToken: 'page2',
  }),
  page2: ListBuildersResponse.fromPartial({
    builders: builderItems.slice(10, 20),
    nextPageToken: 'page3',
  }),
  page3: ListBuildersResponse.fromPartial({
    builders: builderItems.slice(20, 25),
    nextPageToken: '',
  }),
};

describe('<BuilderGroupPage />', () => {
  let listBuildersSpy: jest.SpiedFunction<
    MiloInternalClientImpl['ListBuilders']
  >;
  let builderTableMock: jest.MockedFunction<typeof FilterableBuilderTable>;

  beforeEach(() => {
    jest.useFakeTimers();
    builderTableMock = jest.mocked(FilterableBuilderTable);
    listBuildersSpy = jest
      .spyOn(MiloInternalClientImpl.prototype, 'ListBuilders')
      .mockImplementation(async (req) => pages[req.pageToken]);
  });

  afterEach(() => {
    jest.useRealTimers();
    listBuildersSpy.mockReset();
    builderTableMock.mockReset();
  });

  it('should render correctly', async () => {
    render(
      <FakeContextProvider
        mountedPath="/p/:project/g/:group/builders"
        routerOptions={{
          initialEntries: ['/p/proj/g/a_group/builders'],
        }}
      >
        <BuilderGroupPage />
      </FakeContextProvider>,
    );

    // Load all pages automatically.
    await act(() => jest.runAllTimersAsync());
    await act(() => jest.runAllTimersAsync());
    expect(listBuildersSpy).toHaveBeenCalledTimes(3);

    // No more page loading.
    await act(() => jest.runAllTimersAsync());
    expect(listBuildersSpy).toHaveBeenCalledTimes(3);

    expect(builderTableMock).toHaveBeenCalledWith(
      {
        builders,
        maxBatchSize: expect.anything(),
      },
      undefined,
    );
  });
});
