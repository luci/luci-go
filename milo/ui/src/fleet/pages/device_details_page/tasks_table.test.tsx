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
// limitations under the License.n
import { render, screen, waitFor } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import {
  mockErrorListingBots,
  mockListBots,
} from '@/fleet/testing_tools/mocks/bots_mock';
import {
  mockErrorListingBotTasks,
  mockListBotTasks,
} from '@/fleet/testing_tools/mocks/tasks_mock';
import {
  BotInfo,
  TaskResultResponse,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';

import { Tasks } from './tasks_table';

// TODO: b/404534941 Use `DEVICE_TASKS_SWARMING_HOST` instead.
const swarmingHost = SETTINGS.swarming.defaultHost;

describe('<Tasks />', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('warns when bot request errors', async () => {
    const errorMsg = 'Test ListBots GRPC error';
    mockErrorListingBots(errorMsg);

    render(
      <FakeContextProvider>
        <Tasks dutId="A1234" swarmingHost={swarmingHost} />
      </FakeContextProvider>,
    );

    await waitFor(() => expect(screen.getByText(errorMsg)).toBeVisible());
  });

  it('warns when no bot ID is found', async () => {
    mockListBots([]);

    render(
      <FakeContextProvider>
        <Tasks dutId="dut1331" swarmingHost={swarmingHost} />
      </FakeContextProvider>,
    );

    await waitFor(() =>
      expect(screen.getByText('Bot not found!')).toBeVisible(),
    );
  });

  it('warns when tasks request errors', async () => {
    const errorMsg = 'Test ListTasks GRPC error';
    mockListBots([BotInfo.fromPartial({ botId: 'bot-1' })]);
    mockErrorListingBotTasks(errorMsg);

    render(
      <FakeContextProvider>
        <Tasks dutId="A1234" swarmingHost={swarmingHost} />
      </FakeContextProvider>,
    );

    await waitFor(() => expect(screen.getByText(errorMsg)).toBeVisible());
  });

  it('renders tasks list', async () => {
    mockListBots([BotInfo.fromPartial({ botId: 'bot-1' })]);
    mockListBotTasks([
      TaskResultResponse.fromPartial({ taskId: '1', name: 'task-1' }),
      TaskResultResponse.fromPartial({ taskId: '2', name: 'task-2' }),
    ]);

    render(
      <FakeContextProvider>
        <Tasks dutId="A1234" swarmingHost={swarmingHost} />
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('task-1')).toBeVisible();
      expect(screen.getByText('task-2')).toBeVisible();
    });
  });

  it('informs when no tasks found', async () => {
    const dutId = 'A1234';
    const botId = 'bot-1';
    mockListBots([BotInfo.fromPartial({ botId })]);
    mockListBotTasks([]);

    render(
      <FakeContextProvider>
        <Tasks dutId={dutId} swarmingHost={swarmingHost} />
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('No tasks found')).toBeVisible();
      expect(screen.getByText(dutId)).toBeVisible();
      expect(screen.getByText(botId)).toBeVisible();
    });
  });
});
