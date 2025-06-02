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
import type { MockOptionsMethodPost } from 'fetch-mock';
import fetchMock from 'fetch-mock-jest';

import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import {
  BotInfo,
  BotInfoListResponse,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';

import { useCurrentTasks } from './use_current_tasks';

const LIST_BOTS_ENDPOINT = `https://${DEVICE_TASKS_SWARMING_HOST}/prpc/swarming.v2.Bots/ListBots`;

const createDevice = (dutId: string): Device =>
  Device.fromPartial({ dutId: dutId });

function mockSwarmingListBotsSuccess(
  bots: BotInfo[],
  options: MockOptionsMethodPost = {},
) {
  const response = BotInfoListResponse.fromPartial({ items: bots });
  fetchMock.post(
    LIST_BOTS_ENDPOINT,
    {
      headers: { 'X-Prpc-Grpc-Code': '0' },
      body: ")]}'\n" + JSON.stringify(BotInfoListResponse.toJSON(response)),
    },
    options,
  );
}

function mockSwarmingListBotsError(errorMessage: string) {
  fetchMock.post(LIST_BOTS_ENDPOINT, {
    headers: { 'X-Prpc-Grpc-Code': '2' },
    body: errorMessage,
  });
}

describe('useCurrentTasks', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('should return an empty map and no error for no devices', async () => {
    const { result } = renderHook(() => useCurrentTasks([]), {
      wrapper: FakeContextProvider,
    });

    // isPending should be false immediately if there are no queries to make.
    expect(result.current.isPending).toBe(false);
    expect(result.current.map.size).toBe(0);
    expect(result.current.isError).toBe(false);
    expect(fetchMock.calls(LIST_BOTS_ENDPOINT).length).toBe(0);
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

    const { result } = renderHook(() => useCurrentTasks(devices), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(result.current.map.size).toBe(1);
    expect(result.current.map.get('dut-1')).toBe('task-123');
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
    fetchMock.postOnce(
      (url, opts) =>
        url === LIST_BOTS_ENDPOINT &&
        (opts.body as string).includes('"value":"dut-1|dut-2"'),
      {
        headers: { 'X-Prpc-Grpc-Code': '0' },
        body:
          ")]}'\n" +
          JSON.stringify(BotInfoListResponse.toJSON(firstChunkResponse)),
      },
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
    fetchMock.postOnce(
      (url, opts) =>
        url === LIST_BOTS_ENDPOINT &&
        (opts.body as string).includes('"value":"dut-3"'),
      {
        headers: { 'X-Prpc-Grpc-Code': '0' },
        body:
          ")]}'\n" +
          JSON.stringify(BotInfoListResponse.toJSON(secondChunkResponse)),
      },
    );

    const { result } = renderHook(() => useCurrentTasks(devices, chunkSize), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(result.current.map.size).toBe(3);
    expect(result.current.map.get('dut-1')).toBe('task-1');
    expect(result.current.map.get('dut-2')).toBe('task-2');
    expect(result.current.map.get('dut-3')).toBe('task-3');
    expect(result.current.isError).toBe(false);
    expect(fetchMock.calls().length).toBe(2); // Ensure two separate calls were made
  });

  it('should handle API errors gracefully', async () => {
    const devices = [createDevice('dut-error')];
    mockSwarmingListBotsError('Swarming API error: Timeout');

    const { result } = renderHook(() => useCurrentTasks(devices), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(result.current.map.size).toBe(0);
    expect(result.current.isError).toBe(true);
    expect(result.current.error).toBeInstanceOf(Error);
  });
});
