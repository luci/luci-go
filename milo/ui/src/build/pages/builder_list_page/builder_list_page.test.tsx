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

import { act, render } from '@testing-library/react';

import {
  BuilderID,
  BuilderItem,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import {
  BuildersClientImpl,
  ListBuildersResponse,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuilderListPage } from './builder_list_page';
import { BuilderTable } from './builder_table';

jest.mock('./builder_table', () =>
  self.createSelectiveMockFromModule<typeof import('./builder_table')>(
    './builder_table',
    ['BuilderTable'],
  ),
);

const builders = Array(25)
  .fill(0)
  .map((_, i) =>
    BuilderID.fromPartial({
      project: 'proj',
      bucket: 'bucket',
      builder: `builder${i}`,
    }),
  );

const builderItems = builders.map((id) => BuilderItem.fromPartial({ id }));

const pages: { [key: string]: ListBuildersResponse } = {
  '': ListBuildersResponse.fromPartial({
    builders: Object.freeze(builderItems.slice(0, 10)),
    nextPageToken: 'page2',
  }),
  page2: ListBuildersResponse.fromPartial({
    builders: Object.freeze(builderItems.slice(10, 20)),
    nextPageToken: 'page3',
  }),
  page3: ListBuildersResponse.fromPartial({
    builders: Object.freeze(builderItems.slice(20, 25)),
    nextPageToken: '',
  }),
};

describe('<BuilderListPage>', () => {
  let listBuildersSpy: jest.SpiedFunction<BuildersClientImpl['ListBuilders']>;
  let builderTableMock: jest.MockedFunction<typeof BuilderTable>;

  beforeEach(() => {
    jest.useFakeTimers();
    builderTableMock = jest.mocked(BuilderTable);
    listBuildersSpy = jest
      .spyOn(BuildersClientImpl.prototype, 'ListBuilders')
      .mockImplementation(async (req) => pages[req.pageToken]);
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('should render correctly', async () => {
    render(
      <FakeContextProvider
        mountedPath="/p/:project/builders"
        routerOptions={{
          initialEntries: ['/p/proj/builders?numOfBuilds=12'],
        }}
      >
        <BuilderListPage />
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
        builders: builders,
        numOfBuilds: 12,
      },
      expect.anything(),
    );
  });
});
