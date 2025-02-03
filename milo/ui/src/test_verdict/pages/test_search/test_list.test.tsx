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

import { render, cleanup } from '@testing-library/react';
import { act } from 'react';

import {
  QueryTestsRequest,
  QueryTestsResponse,
  TestHistoryClientImpl,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_history.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestList } from './test_list';

describe('TestList', () => {
  let queryTestsMock: jest.SpyInstance;

  beforeEach(() => {
    jest.useFakeTimers();
    queryTestsMock = jest
      .spyOn(TestHistoryClientImpl.prototype, 'QueryTests')
      .mockResolvedValue(
        QueryTestsResponse.fromPartial({
          testIds: ['test-id-1', 'test-id-2'],
        }),
      );
  });
  afterEach(() => {
    cleanup();
    queryTestsMock.mockRestore();
    jest.useRealTimers();
  });

  it('should NOT load the first page of test when search query is empty', async () => {
    render(
      <FakeContextProvider>
        <TestList project="chromium" searchQuery="" />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(queryTestsMock).not.toHaveBeenCalled();
  });

  it('should load the first page of test when search query is NOT empty', async () => {
    render(
      <FakeContextProvider>
        <TestList project="chromium" searchQuery="test-id" />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    // This will be called twice, once for case sensitive and one for insensitive.
    expect(queryTestsMock).toHaveBeenCalledTimes(2);
    expect(queryTestsMock).toHaveBeenCalledWith(
      QueryTestsRequest.fromPartial({
        project: 'chromium',
        testIdSubstring: 'test-id',
      }),
    );
  });
});
