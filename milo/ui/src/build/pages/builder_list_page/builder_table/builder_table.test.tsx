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

import { act, render, screen } from '@testing-library/react';
import { VirtuosoMockContext } from 'react-virtuoso';

import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuilderTable } from './builder_table';

const builders = Array(1000)
  .fill(0)
  .map((_, i) =>
    BuilderID.fromPartial({
      project: 'proj',
      bucket: 'bucket',
      builder: `builder${i}`,
    }),
  );

describe('<BuilderListPage>', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('should not render items that are out of bound', async () => {
    render(
      <FakeContextProvider>
        <VirtuosoMockContext.Provider
          value={{ viewportHeight: 400, itemHeight: 40 }}
        >
          <BuilderTable builders={builders} numOfBuilds={10} />
        </VirtuosoMockContext.Provider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());
    expect(screen.queryByText('proj/bucket/builder0')).toBeInTheDocument();
    expect(screen.queryByText('proj/bucket/builder5')).toBeInTheDocument();
    expect(screen.queryByText('proj/bucket/builder10')).not.toBeInTheDocument();
  });
});
