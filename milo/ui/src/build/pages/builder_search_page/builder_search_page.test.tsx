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

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from '@testing-library/react';

import { UiPage } from '@/common/constants/view';
import {
  ListBuildersRequest,
  ListBuildersResponse,
  MiloInternalClientImpl,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuilderSearch } from './builder_search_page';

describe('BuilderSearch', () => {
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
      .mockImplementation((_: ListBuildersRequest) => {
        return Promise.resolve(
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
      });

    render(
      <FakeContextProvider
        pageMeta={{
          selectedPage: UiPage.Builders,
        }}
      >
        <BuilderSearch />
      </FakeContextProvider>,
    );

    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'builder' },
    });
    act(() => jest.advanceTimersByTime(10));
    expect(screen.queryAllByTestId('builder-data').length).toBe(0);

    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'builder-id' },
    });
    act(() => jest.advanceTimersByTime(10));
    expect(screen.queryAllByTestId('builder-data').length).toBe(0);

    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'builder-id-with-suffix' },
    });
    act(() => jest.advanceTimersByTime(300));
    expect(screen.queryAllByTestId('builder-data').length).toBe(0);

    await screen.findByText('chromium/test_bucket');
    expect(screen.getAllByTestId('builder-data').length).toBe(1);
    expect(screen.getByText('builder-id-with-suffix')).toBeInTheDocument();

    // Using another filter.
    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'another-builder' },
    });
    act(() => jest.advanceTimersByTime(10));
    expect(screen.getAllByTestId('builder-data').length).toBe(1);
    expect(screen.getByText('builder-id-with-suffix')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'another-builder-id' },
    });
    act(() => jest.runAllTimers());
    expect(screen.getAllByTestId('builder-data').length).toBe(1);
    expect(screen.getByText('another-builder-id')).toBeInTheDocument();
  });
});
