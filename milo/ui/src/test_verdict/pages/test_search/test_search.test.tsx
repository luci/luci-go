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

import {
  QueryTestsRequest,
  QueryTestsResponse,
  TestHistoryClientImpl,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_history.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestSearch } from './test_search';

describe('TestSearch', () => {
  beforeEach(() => {
    jest
      .spyOn(TestHistoryClientImpl.prototype, 'QueryTests')
      .mockImplementation((req: QueryTestsRequest) => {
        if (req.testIdSubstring === 'query') {
          return Promise.resolve(
            QueryTestsResponse.fromPartial({
              testIds: ['test_id_1', 'test_id_2'],
            }),
          );
        } else {
          return Promise.reject(new Error('unknown test id'));
        }
      });
    jest.useFakeTimers();
  });
  afterEach(() => {
    cleanup();
    jest.useRealTimers();
    jest.resetAllMocks();
  });

  it('should read params from URL', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/?q=query'],
        }}
      >
        <TestSearch />
      </FakeContextProvider>,
    );

    // Ensure that the filters are not overwritten after all event hooks are
    // executed.
    await act(async () => await jest.runAllTimersAsync());

    await screen.findByText('test_id_1');
    expect(screen.getByText('test_id_2')).toBeInTheDocument();
  });

  it('should sync params with URL', async () => {
    render(
      <FakeContextProvider>
        <TestSearch />
      </FakeContextProvider>,
    );
    fireEvent.change(screen.getByTestId('search-input'), {
      target: { value: 'query' },
    });

    await screen.findByText('test_id_1');
    expect(screen.getByText('test_id_2')).toBeInTheDocument();
  });

  it('should throttle requests', async () => {
    jest
      .spyOn(TestHistoryClientImpl.prototype, 'QueryTests')
      .mockImplementation((req: QueryTestsRequest) => {
        const result = ['test_id_1', 'test_id_2'].filter((id) =>
          id.startsWith(req.testIdSubstring),
        );
        return Promise.resolve(
          QueryTestsResponse.fromPartial({
            testIds: result,
          }),
        );
      });

    render(
      <FakeContextProvider>
        <TestSearch />
      </FakeContextProvider>,
    );
    fireEvent.change(screen.getByTestId('search-input'), {
      target: { value: 'test_id' },
    });
    expect(screen.queryByText('test_id_1')).not.toBeInTheDocument();
    expect(screen.queryByText('test_id_2')).not.toBeInTheDocument();
    fireEvent.change(screen.getByTestId('search-input'), {
      target: { value: 'test_id_2' },
    });
    act(() => jest.advanceTimersByTime(600));
    await screen.findByText('test_id_2');
    expect(screen.queryByText('test_id_1')).not.toBeInTheDocument();
  });
});
