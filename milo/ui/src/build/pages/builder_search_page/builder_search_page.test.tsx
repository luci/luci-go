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

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { act } from 'react';
import { VirtuosoMockContext } from 'react-virtuoso';

import { UiPage } from '@/common/constants/view';
import {
  ListBuildersRequest,
  ListBuildersResponse,
  MiloInternalClientImpl,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuilderSearchPage } from './builder_search_page';

describe('<BuilderSearchPage />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });
  afterEach(() => {
    cleanup();
    jest.useRealTimers();
    jest.resetAllMocks();
  });

  it('should throttle search requests', async () => {
    jest
      .spyOn(MiloInternalClientImpl.prototype, 'ListBuilders')
      .mockImplementation(async (_: ListBuildersRequest) =>
        ListBuildersResponse.fromPartial({
          builders: Object.freeze([
            {
              id: {
                bucket: 'test_bucket',
                builder: 'builder',
                project: 'chromium',
              },
            },
            {
              id: {
                bucket: 'test_bucket',
                builder: 'builder-id',
                project: 'chromium',
              },
            },
            {
              id: {
                bucket: 'test_bucket',
                builder: 'builder-id-with-suffix',
                project: 'chromium',
              },
            },
            {
              id: {
                bucket: 'test_bucket',
                builder: 'another-builder',
                project: 'chromium',
              },
            },
            {
              id: {
                bucket: 'test_bucket',
                builder: 'another-builder-id',
                project: 'chromium',
              },
            },
          ]),
        }),
      );

    render(
      <FakeContextProvider
        pageMeta={{
          selectedPage: UiPage.Builders,
        }}
      >
        <VirtuosoMockContext.Provider
          value={{ viewportHeight: 1000, itemHeight: 1 }}
        >
          <BuilderSearchPage />
        </VirtuosoMockContext.Provider>
      </FakeContextProvider>,
    );

    fireEvent.change(screen.getByTestId('search-input'), {
      target: { value: 'builder' },
    });
    await act(() => jest.advanceTimersByTimeAsync(10));
    expect(screen.queryAllByTestId('builder-data').length).toBe(5);

    fireEvent.change(screen.getByTestId('search-input'), {
      target: { value: 'builder-id' },
    });
    await act(() => jest.advanceTimersByTimeAsync(10));
    expect(screen.queryAllByTestId('builder-data').length).toBe(5);

    fireEvent.change(screen.getByTestId('search-input'), {
      target: { value: 'builder-id-with-suffix' },
    });
    await act(() => jest.advanceTimersByTimeAsync(200));
    expect(screen.queryAllByTestId('builder-data').length).toBe(5);

    await act(() => jest.advanceTimersByTimeAsync(200));
    expect(screen.getAllByTestId('builder-data').length).toBe(1);
    expect(screen.getByText('builder-id-with-suffix')).toBeInTheDocument();

    // Using another filter.
    fireEvent.change(screen.getByTestId('search-input'), {
      target: { value: 'another-builder' },
    });
    await act(() => jest.advanceTimersByTimeAsync(10));
    expect(screen.getAllByTestId('builder-data').length).toBe(1);
    expect(screen.getByText('builder-id-with-suffix')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('search-input'), {
      target: { value: 'another-builder-id' },
    });
    act(() => jest.runAllTimers());
    expect(screen.getAllByTestId('builder-data').length).toBe(1);
    expect(screen.getByText('another-builder-id')).toBeInTheDocument();
  });
});
