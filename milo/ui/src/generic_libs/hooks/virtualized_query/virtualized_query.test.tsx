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

import { renderHook } from '@testing-library/react';
import { act } from 'react';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  UseVirtualizedQueryOption,
  UseVirtualizedQueryResult,
  useVirtualizedQuery,
} from './virtualized_query';

describe('useVirtualizedQuery', () => {
  let queryFn: jest.Mock<
    readonly string[],
    [start: number, end: number],
    unknown
  >;

  beforeEach(() => {
    jest.useFakeTimers();
    queryFn = jest.fn((start: number, end: number) =>
      Array(end - start)
        .fill(undefined)
        .map((_, i) => ((start + i) * 10).toString()),
    );
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it('works with regular setup', async () => {
    const { result } = renderHook<
      UseVirtualizedQueryResult<string>,
      UseVirtualizedQueryOption<readonly string[], string>
    >(useVirtualizedQuery, {
      initialProps: {
        rangeBoundary: [100, 200],
        interval: 20,
        initRange: [120, 160],
        genQuery(start, end) {
          return {
            queryKey: [start, end],
            queryFn: () => queryFn(start, end),
            staleTime: Infinity,
          };
        },
        delayMs: 100,
      },
      wrapper: FakeContextProvider,
    });

    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(2);
    expect(queryFn).toHaveBeenCalledWith(120, 140);
    expect(queryFn).toHaveBeenCalledWith(140, 160);
    expect(result.current.get(100)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(120)).toMatchObject({
      data: '1200',
      status: 'success',
    });
    expect(result.current.get(140)).toMatchObject({
      data: '1400',
      status: 'success',
    });
    expect(result.current.get(160)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
  });

  it('can handle query range update', async () => {
    const { result } = renderHook<
      UseVirtualizedQueryResult<string>,
      UseVirtualizedQueryOption<readonly string[], string>
    >(useVirtualizedQuery, {
      initialProps: {
        rangeBoundary: [100, 200],
        interval: 20,
        initRange: [120, 160],
        genQuery(start, end) {
          return {
            queryKey: [start, end],
            queryFn: () => queryFn(start, end),
            staleTime: Infinity,
          };
        },
        delayMs: 100,
      },
      wrapper: FakeContextProvider,
    });

    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(2);

    // Switch to a new range.
    act(() => result.current.setRange([100, 140]));

    // Query not sent yet.
    await act(() => jest.advanceTimersByTimeAsync(50));
    expect(queryFn).toHaveBeenCalledTimes(2);
    expect(queryFn).not.toHaveBeenCalledWith(100, 120);
    expect(result.current.get(80)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(100)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    // Items in `[start, end) ∩ [prevStart, prevEnd)` is still accessible.
    expect(result.current.get(120)).toMatchObject({
      data: '1200',
      status: 'success',
    });
    expect(result.current.get(140)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });

    // Query is sent.
    await act(() => jest.advanceTimersByTimeAsync(100));
    await act(() => jest.advanceTimersByTimeAsync(0));
    expect(queryFn).toHaveBeenCalledTimes(3);
    expect(queryFn).toHaveBeenCalledWith(100, 120);
    expect(result.current.get(80)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(100)).toMatchObject({
      data: '1000',
      status: 'success',
    });
    // Items in `[start, end) ∩ [prevStart, prevEnd)` is still accessible.
    expect(result.current.get(120)).toMatchObject({
      data: '1200',
      status: 'success',
    });
    expect(result.current.get(140)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });

    // Switch back to the previous range.
    act(() => result.current.setRange([120, 160]));

    expect(queryFn).toHaveBeenCalledTimes(3);
    expect(result.current.get(80)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(100)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    // Items in `[start, end) ∩ [prevStart, prevEnd)` is still accessible.
    expect(result.current.get(120)).toMatchObject({
      data: '1200',
      status: 'success',
    });
    // Items in `[start, end) ∩ [prevStart, prevEnd)` is still accessible.
    expect(result.current.get(140)).toMatchObject({
      data: '1400',
      status: 'success',
    });
  });

  it('can debounce query range update', async () => {
    const { result } = renderHook<
      UseVirtualizedQueryResult<string>,
      UseVirtualizedQueryOption<readonly string[], string>
    >(useVirtualizedQuery, {
      initialProps: {
        rangeBoundary: [20, 200],
        interval: 20,
        initRange: [140, 180],
        genQuery(start, end) {
          return {
            queryKey: [start, end],
            queryFn: () => queryFn(start, end),
            staleTime: Infinity,
          };
        },
        delayMs: 100,
      },
      wrapper: FakeContextProvider,
    });

    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(2);

    // Switch to a new range.
    act(() => result.current.setRange([120, 140]));

    // Query not sent yet.
    await act(() => jest.advanceTimersByTimeAsync(60));
    expect(queryFn).toHaveBeenCalledTimes(2);

    // Switch to yet another new range.
    act(() => result.current.setRange([85, 115]));

    // Query not sent yet.
    await act(() => jest.advanceTimersByTimeAsync(60));
    expect(queryFn).toHaveBeenCalledTimes(2);

    // Switch to yet another new range. But the new range does not change the
    // chunks to be loaded.
    act(() => result.current.setRange([82, 112]));

    // Query is sent 100ms after the last effective query range update.
    await act(() => jest.advanceTimersByTimeAsync(60));
    await act(() => jest.advanceTimersByTimeAsync(0));
    expect(queryFn).toHaveBeenCalledTimes(4);
    expect(queryFn).toHaveBeenCalledWith(80, 100);
    expect(queryFn).toHaveBeenCalledWith(100, 120);
    expect(result.current.get(70)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(80)).toMatchObject({
      data: '800',
      status: 'success',
    });
    expect(result.current.get(117)).toMatchObject({
      data: '1170',
      status: 'success',
    });
    expect(result.current.get(123)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
  });

  it('debounce chunks individually', async () => {
    const { result } = renderHook<
      UseVirtualizedQueryResult<string>,
      UseVirtualizedQueryOption<readonly string[], string>
    >(useVirtualizedQuery, {
      initialProps: {
        rangeBoundary: [20, 200],
        interval: 20,
        initRange: [140, 180],
        genQuery(start, end) {
          return {
            queryKey: [start, end],
            queryFn: () => queryFn(start, end),
            staleTime: Infinity,
          };
        },
        delayMs: 100,
      },
      wrapper: FakeContextProvider,
    });

    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(2);

    // Switch to a new range.
    act(() => result.current.setRange([25, 65]));

    // Query not sent yet.
    await act(() => jest.advanceTimersByTimeAsync(60));
    expect(queryFn).toHaveBeenCalledTimes(2);

    // Switch to yet another new range.
    act(() => result.current.setRange([55, 95]));

    // Query for [40, 80) is sent because it's active for more than 100ms.
    await act(() => jest.advanceTimersByTimeAsync(60));
    expect(queryFn).toHaveBeenCalledTimes(4);
    expect(queryFn).toHaveBeenCalledWith(40, 60);
    expect(queryFn).toHaveBeenCalledWith(60, 80);

    // Switch to yet another new range.
    act(() => result.current.setRange([65, 105]));

    // Query for [80, 100) is sent because it's active for more than 100ms.
    await act(() => jest.advanceTimersByTimeAsync(60));
    expect(queryFn).toHaveBeenCalledTimes(5);
    expect(queryFn).toHaveBeenCalledWith(80, 100);

    // Switch to yet another new range. But the new range does not change the
    // chunks to be loaded.
    act(() => result.current.setRange([70, 110]));
    await act(() => jest.advanceTimersByTimeAsync(60));
    expect(queryFn).toHaveBeenCalledTimes(6);
    expect(queryFn).toHaveBeenCalledWith(100, 120);
  });

  // TODO(b/416138280): fix test.
  // eslint-disable-next-line jest/no-disabled-tests
  it.skip('does not discard query range update even when chunk range is unchanged', async () => {
    const { result, rerender } = renderHook<
      UseVirtualizedQueryResult<string>,
      UseVirtualizedQueryOption<readonly string[], string>
    >(useVirtualizedQuery, {
      initialProps: {
        rangeBoundary: [100, 200],
        interval: 20,
        initRange: [100, 140],
        genQuery(start, end) {
          return {
            queryKey: [start, end],
            queryFn: () => queryFn(start, end),
            staleTime: Infinity,
          };
        },
        delayMs: 100,
      },
      wrapper: FakeContextProvider,
    });

    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(2);

    // Switch to a new range.
    act(() => result.current.setRange([80, 140]));

    // No new query.
    await act(() => jest.advanceTimersByTimeAsync(150));
    expect(queryFn).toHaveBeenCalledTimes(2);

    // Expand the bound to include the extra range queried previously.
    rerender({
      rangeBoundary: [60, 200],
      interval: 20,
      initRange: [100, 140],
      genQuery(start, end) {
        return {
          queryKey: [start, end],
          queryFn: () => queryFn(start, end),
          staleTime: Infinity,
        };
      },
      delayMs: 100,
    });
    await act(() => jest.advanceTimersByTimeAsync(0));
    expect(queryFn).toHaveBeenCalledTimes(3);
    expect(queryFn).toHaveBeenCalledWith(80, 100);
    expect(result.current.get(70)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(80)).toMatchObject({
      data: '800',
      status: 'success',
    });
  });

  it('can align query range', async () => {
    const { result } = renderHook<
      UseVirtualizedQueryResult<string>,
      UseVirtualizedQueryOption<readonly string[], string>
    >(useVirtualizedQuery, {
      initialProps: {
        rangeBoundary: [100, 200],
        interval: 20,
        // The range will be expanded to [120, 160].
        initRange: [130, 150],
        genQuery(start, end) {
          return {
            queryKey: [start, end],
            queryFn: () => queryFn(start, end),
            staleTime: Infinity,
          };
        },
        delayMs: 100,
      },
      wrapper: FakeContextProvider,
    });

    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(2);
    expect(queryFn).toHaveBeenCalledWith(120, 140);
    expect(queryFn).toHaveBeenCalledWith(140, 160);
    expect(result.current.get(100)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(120)).toMatchObject({
      data: '1200',
      status: 'success',
    });
    expect(result.current.get(140)).toMatchObject({
      data: '1400',
      status: 'success',
    });
    expect(result.current.get(160)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
  });

  it('can handle unaligned bound', async () => {
    const { result } = renderHook<
      UseVirtualizedQueryResult<string>,
      UseVirtualizedQueryOption<readonly string[], string>
    >(useVirtualizedQuery, {
      initialProps: {
        rangeBoundary: [105, 198],
        interval: 20,
        // The range will be expanded to [120, 160].
        initRange: [130, 150],
        genQuery(start, end) {
          return {
            queryKey: [start, end],
            queryFn: () => queryFn(start, end),
            staleTime: Infinity,
          };
        },
        delayMs: 100,
      },
      wrapper: FakeContextProvider,
    });

    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(2);
    expect(queryFn).toHaveBeenCalledWith(120, 140);
    expect(queryFn).toHaveBeenCalledWith(140, 160);
    expect(result.current.get(100)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(120)).toMatchObject({
      data: '1200',
      status: 'success',
    });
    expect(result.current.get(140)).toMatchObject({
      data: '1400',
      status: 'success',
    });
    expect(result.current.get(160)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
  });

  it('can truncate query range', async () => {
    const { result } = renderHook<
      UseVirtualizedQueryResult<string>,
      UseVirtualizedQueryOption<readonly string[], string>
    >(useVirtualizedQuery, {
      initialProps: {
        rangeBoundary: [100, 200],
        interval: 20,
        // The range will be converted to [100, 160].
        initRange: [80, 150],
        genQuery(start, end) {
          return {
            queryKey: [start, end],
            queryFn: () => queryFn(start, end),
            staleTime: Infinity,
          };
        },
        delayMs: 100,
      },
      wrapper: FakeContextProvider,
    });

    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(3);
    expect(queryFn).toHaveBeenCalledWith(100, 120);
    expect(queryFn).toHaveBeenCalledWith(120, 140);
    expect(queryFn).toHaveBeenCalledWith(140, 160);
    expect(result.current.get(80)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(100)).toMatchObject({
      data: '1000',
      status: 'success',
    });
    expect(result.current.get(120)).toMatchObject({
      data: '1200',
      status: 'success',
    });
    expect(result.current.get(140)).toMatchObject({
      data: '1400',
      status: 'success',
    });
    expect(result.current.get(160)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
  });

  it('can handle unaligned bound with out of start bound query range', async () => {
    const { result } = renderHook<
      UseVirtualizedQueryResult<string>,
      UseVirtualizedQueryOption<readonly string[], string>
    >(useVirtualizedQuery, {
      initialProps: {
        rangeBoundary: [105, 198],
        interval: 20,
        // The range will be aligned to [105, 140].
        initRange: [90, 125],
        genQuery(start, end) {
          return {
            queryKey: [start, end],
            queryFn: () => queryFn(start, end),
            staleTime: Infinity,
          };
        },
        delayMs: 100,
      },
      wrapper: FakeContextProvider,
    });

    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(2);
    expect(queryFn).toHaveBeenCalledWith(105, 120);
    expect(queryFn).toHaveBeenCalledWith(120, 140);
    expect(result.current.get(101)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(120)).toMatchObject({
      data: '1200',
      status: 'success',
    });
    expect(result.current.get(130)).toMatchObject({
      data: '1300',
      status: 'success',
    });
    expect(result.current.get(140)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
  });

  it('can handle unaligned bound with out of end bound query range', async () => {
    const { result } = renderHook<
      UseVirtualizedQueryResult<string>,
      UseVirtualizedQueryOption<readonly string[], string>
    >(useVirtualizedQuery, {
      initialProps: {
        rangeBoundary: [105, 198],
        interval: 20,
        // The range will be aligned to [160, 198].
        initRange: [175, 203],
        genQuery(start, end) {
          return {
            queryKey: [start, end],
            queryFn: () => queryFn(start, end),
            staleTime: Infinity,
          };
        },
        delayMs: 100,
      },
      wrapper: FakeContextProvider,
    });

    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(2);
    expect(queryFn).toHaveBeenCalledWith(160, 180);
    expect(queryFn).toHaveBeenCalledWith(180, 198);
    expect(result.current.get(155)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(162)).toMatchObject({
      data: '1620',
      status: 'success',
    });
    expect(result.current.get(179)).toMatchObject({
      data: '1790',
      status: 'success',
    });
    expect(result.current.get(197)).toMatchObject({
      data: '1970',
      status: 'success',
    });
    expect(result.current.get(202)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
  });

  // TODO(b/416138280): fix test.
  // eslint-disable-next-line jest/no-disabled-tests
  it.skip('can handle bound update', async () => {
    const { result, rerender } = renderHook<
      UseVirtualizedQueryResult<string>,
      UseVirtualizedQueryOption<readonly string[], string>
    >(useVirtualizedQuery, {
      initialProps: {
        rangeBoundary: [100, 200],
        interval: 20,
        // The range will be converted to [100, 160].
        initRange: [80, 150],
        genQuery(start, end) {
          return {
            queryKey: [start, end],
            queryFn: () => queryFn(start, end),
            staleTime: Infinity,
          };
        },
        delayMs: 100,
      },
      wrapper: FakeContextProvider,
    });

    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(3);

    rerender({
      rangeBoundary: [50, 145],
      interval: 20,
      // The range will be converted to [80, 145].
      initRange: [80, 150],
      genQuery(start, end) {
        return {
          queryKey: [start, end],
          queryFn: () => queryFn(start, end),
          staleTime: Infinity,
        };
      },
      delayMs: 100,
    });
    await act(() => jest.advanceTimersByTimeAsync(1));
    expect(queryFn).toHaveBeenCalledTimes(5);
    expect(queryFn).toHaveBeenCalledWith(80, 100);
    expect(queryFn).toHaveBeenCalledWith(140, 145);
    expect(result.current.get(70)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
    expect(result.current.get(80)).toMatchObject({
      data: '800',
      status: 'success',
      fetchStatus: 'idle',
    });
    expect(result.current.get(100)).toMatchObject({
      data: '1000',
      status: 'success',
    });
    expect(result.current.get(144)).toMatchObject({
      data: '1440',
      status: 'success',
    });
    expect(result.current.get(150)).toMatchObject({
      data: undefined,
      status: 'pending',
      fetchStatus: 'idle',
    });
  });
});
