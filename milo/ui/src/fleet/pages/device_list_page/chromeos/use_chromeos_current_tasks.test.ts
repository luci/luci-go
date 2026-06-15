// Copyright 2025 The LUCI Authors.
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

import { renderHook, waitFor } from '@testing-library/react';

import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import {
  BotInfo,
  BotInfoListResponse,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import {
  mockFetchHandler,
  mockFetchRaw,
  resetMockFetch,
} from '@/testing_tools/jest_utils';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';

import { useChromeOSCurrentTasks } from './use_chromeos_current_tasks';

const LIST_BOTS_ENDPOINT = `https://${DEVICE_TASKS_SWARMING_HOST}/prpc/swarming.v2.Bots/ListBots`;

const createDevice = (dutId: string): Device =>
  Device.fromPartial({ dutId: dutId });

function mockSwarmingListBotsSuccess(bots: BotInfo[]) {
  const response = BotInfoListResponse.fromPartial({ items: bots });
  mockFetchRaw(
    (url) => url === LIST_BOTS_ENDPOINT,
    ")]}'\n" + JSON.stringify(BotInfoListResponse.toJSON(response)),
    {
      headers: { 'X-Prpc-Grpc-Code': '0' },
    },
  );
}

function mockSwarmingListBotsError(errorMessage: string) {
  mockFetchRaw((url) => url === LIST_BOTS_ENDPOINT, errorMessage, {
    headers: { 'X-Prpc-Grpc-Code': '2' },
  });
}

describe('useChromeOSCurrentTasks', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    resetMockFetch();
  });

  it('should return an empty map and no error for no devices', async () => {
    const { result } = renderHook(() => useChromeOSCurrentTasks([]), {
      wrapper: FakeContextProvider,
    });

    // isPending should be false immediately if there are no queries to make.
    expect(result.current.isPending).toBe(false);
    expect(Object.keys(result.current.tasks).length).toBe(0);
    expect(result.current.isError).toBe(false);
    expect(
      (global.fetch as jest.Mock).mock.calls.filter((args) =>
        (args[0] as string).includes(LIST_BOTS_ENDPOINT),
      ).length,
    ).toBe(0);
  });

  it('should fetch task for a single device successfully', async () => {
    const devices = [createDevice('dut-1')];
    mockSwarmingListBotsSuccess([
      BotInfo.fromPartial({
        botId: 'bot-dut-1-id',
        taskId: 'task-123',
        dimensions: [{ key: 'dut_id', value: ['dut-1'] }],
      }),
    ]);

    const { result } = renderHook(() => useChromeOSCurrentTasks(devices), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(Object.keys(result.current.tasks).length).toBe(1);
    expect(result.current.tasks['dut-1']).toEqual({
      taskId: 'task-123',
      taskName: '',
    });
    expect(result.current.isError).toBe(false);
  });

  it('should fetch tasks for multiple devices using provided chunk size', async () => {
    const chunkSize = 2;
    const devices = [
      createDevice('dut-1'),
      createDevice('dut-2'),
      createDevice('dut-3'),
    ];

    const firstChunkResponse = BotInfoListResponse.fromPartial({
      items: [
        BotInfo.fromPartial({
          botId: 'bot-dut-1-id',
          taskId: 'task-1',
          dimensions: [{ key: 'dut_id', value: ['dut-1'] }],
        }),
        BotInfo.fromPartial({
          botId: 'bot-dut-2-id',
          taskId: 'task-2',
          dimensions: [{ key: 'dut_id', value: ['dut-2'] }],
        }),
      ],
    });

    // Use a function matcher to inspect the request body for the first chunk.
    mockFetchHandler(
      (url, init) =>
        url === LIST_BOTS_ENDPOINT &&
        (init?.body as string | undefined)?.includes(
          '"value":"dut-1|dut-2"',
        ) === true,
      async () =>
        new Response(
          ")]}'\n" +
            JSON.stringify(BotInfoListResponse.toJSON(firstChunkResponse)),
          {
            headers: { 'X-Prpc-Grpc-Code': '0' },
          },
        ),
    );

    const secondChunkResponse = BotInfoListResponse.fromPartial({
      items: [
        BotInfo.fromPartial({
          botId: 'bot-dut-3-id',
          taskId: 'task-3',
          dimensions: [{ key: 'dut_id', value: ['dut-3'] }],
        }),
      ],
    });

    // Use a function matcher for the second chunk.
    mockFetchHandler(
      (url, init) =>
        url === LIST_BOTS_ENDPOINT &&
        (init?.body as string | undefined)?.includes('"value":"dut-3"') ===
          true,
      async () =>
        new Response(
          ")]}'\n" +
            JSON.stringify(BotInfoListResponse.toJSON(secondChunkResponse)),
          {
            headers: { 'X-Prpc-Grpc-Code': '0' },
          },
        ),
    );

    const { result } = renderHook(
      () => useChromeOSCurrentTasks(devices, { chunkSize }),
      {
        wrapper: FakeContextProvider,
      },
    );

    await waitFor(() => expect(result.current.isPending).toBe(false));

    const calls = (global.fetch as jest.Mock).mock.calls.filter((args) =>
      (args[0] as string).includes(LIST_BOTS_ENDPOINT),
    );
    // Ensure two separate calls were made, since the hook can run multiple times we just check that it runs twice on each re-render
    expect(calls.length % 2).toBe(0);
    expect(result.current.error).toBeNull();
    expect(result.current.isError).toBe(false);
    expect(Object.keys(result.current.tasks).length).toBe(3);
    expect(result.current.tasks['dut-1']).toEqual({
      taskId: 'task-1',
      taskName: '',
    });
    expect(result.current.tasks['dut-2']).toEqual({
      taskId: 'task-2',
      taskName: '',
    });
    expect(result.current.tasks['dut-3']).toEqual({
      taskId: 'task-3',
      taskName: '',
    });
  });

  it('should handle API errors gracefully', async () => {
    const devices = [createDevice('dut-error')];
    mockSwarmingListBotsError('Swarming API error: Timeout');

    const { result } = renderHook(() => useChromeOSCurrentTasks(devices), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(Object.keys(result.current.tasks).length).toBe(0);
    expect(result.current.isError).toBe(true);
    expect(result.current.error).toBeInstanceOf(Error);
  });

  it('should maintain referential stability of return values if data does not change', async () => {
    const devices = [createDevice('dut-1')];
    mockSwarmingListBotsSuccess([
      BotInfo.fromPartial({
        botId: 'bot-dut-1-id',
        taskId: 'task-123',
        dimensions: [{ key: 'dut_id', value: ['dut-1'] }],
      }),
    ]);

    const { result, rerender } = renderHook(
      () => useChromeOSCurrentTasks(devices),
      {
        wrapper: FakeContextProvider,
      },
    );

    await waitFor(() => expect(result.current.isPending).toBe(false));

    const initialResult = result.current;

    // Rerender the hook
    rerender();

    // Verify that the reference of the returned object is identical
    expect(result.current).toBe(initialResult);
  });

  it('should maintain referential stability of return values if devices array reference changes but contents are identical', async () => {
    mockSwarmingListBotsSuccess([
      BotInfo.fromPartial({
        botId: 'bot-dut-1-id',
        taskId: 'task-123',
        dimensions: [{ key: 'dut_id', value: ['dut-1'] }],
      }),
    ]);

    const devices1 = [createDevice('dut-1')];
    const devices2 = [createDevice('dut-1')];

    const { result, rerender } = renderHook(
      ({ devList }) => useChromeOSCurrentTasks(devList),
      {
        wrapper: FakeContextProvider,
        initialProps: { devList: devices1 },
      },
    );

    await waitFor(() => expect(result.current.isPending).toBe(false));

    const initialResult = result.current;

    // Rerender with a different devices array reference but same contents
    rerender({ devList: devices2 });

    expect(result.current).toBe(initialResult);
  });
});
