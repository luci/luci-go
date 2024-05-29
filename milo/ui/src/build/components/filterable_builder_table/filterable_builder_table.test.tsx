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

import { BuilderTable } from '@/build/components/builder_table';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { FilterableBuilderTable } from './filterable_builder_table';

jest.mock('@/build/components/builder_table', () =>
  self.createSelectiveMockFromModule<
    typeof import('@/build/components/builder_table')
  >('@/build/components/builder_table', ['BuilderTable']),
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

describe('<FilterableBuilderTable />', () => {
  let builderTableMock: jest.MockedFunction<typeof BuilderTable>;

  beforeEach(() => {
    jest.useFakeTimers();
    builderTableMock = jest.mocked(BuilderTable);
  });

  afterEach(() => {
    jest.useRealTimers();
    builderTableMock.mockReset();
  });

  it('should filter builders', async () => {
    render(
      <FakeContextProvider
        mountedPath="/"
        routerOptions={{
          initialEntries: ['/?q=1/builder1'],
        }}
      >
        <FilterableBuilderTable builders={builders} />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(builderTableMock).toHaveBeenCalledWith(
      {
        builders: builders.filter(
          (b) => b.bucket.endsWith('1') && b.builder.startsWith('builder1'),
        ),
        numOfBuilds: 25,
      },
      expect.anything(),
    );
  });
});
