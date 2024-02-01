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

import { render, cleanup, act } from '@testing-library/react';

import { useInfinitePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import { TestHistoryService } from '@/common/services/luci_analysis';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestList } from './test_list';

jest.mock('@/common/hooks/legacy_prpc_query', () => {
  return createSelectiveSpiesFromModule<
    typeof import('@/common/hooks/legacy_prpc_query')
  >('@/common/hooks/legacy_prpc_query', ['useInfinitePrpcQuery']);
});

describe('TestList', () => {
  let useInfinitePrpcQuerySpy: jest.MockedFunction<typeof useInfinitePrpcQuery>;
  let queryTestsMock: jest.SpyInstance;

  beforeEach(() => {
    jest.useFakeTimers();
    useInfinitePrpcQuerySpy = jest.mocked(useInfinitePrpcQuery);
    queryTestsMock = jest
      .spyOn(TestHistoryService.prototype, 'queryTests')
      .mockResolvedValue({
        testIds: ['test-id-1', 'test-id-2'],
      });
  });
  afterEach(() => {
    cleanup();
    useInfinitePrpcQuerySpy.mockClear();
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

    expect(useInfinitePrpcQuerySpy).toHaveBeenCalledWith({
      host: SETTINGS.luciAnalysis.host,
      Service: TestHistoryService,
      method: 'queryTests',
      request: { project: 'chromium', testIdSubstring: '' },
      options: {
        enabled: false,
      },
    });
    expect(queryTestsMock).not.toHaveBeenCalled();
  });

  it('should load the first page of test when search query is NOT empty', async () => {
    render(
      <FakeContextProvider>
        <TestList project="chromium" searchQuery="test-id" />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(useInfinitePrpcQuerySpy).toHaveBeenCalledWith({
      host: SETTINGS.luciAnalysis.host,
      Service: TestHistoryService,
      method: 'queryTests',
      request: { project: 'chromium', testIdSubstring: 'test-id' },
      options: {
        enabled: true,
      },
    });
    expect(queryTestsMock).toHaveBeenCalled();
  });
});
