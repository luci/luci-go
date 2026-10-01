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

import { SettingsProvider } from '@/fleet/context/providers';
import {
  mockErrorListingBots,
  mockListBots,
} from '@/fleet/testing_tools/mocks/bots_mock';
import {
  mockErrorListingBotTasks,
  mockErrorListingTaskHistory,
  mockListBotTasks,
  mockListTaskHistory,
} from '@/fleet/testing_tools/mocks/tasks_mock';
import {
  TaskHistoryItem,
  TaskSource,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import {
  BotInfo,
  TaskResultResponse,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { resetMockFetch } from '@/testing_tools/jest_utils';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';

import { Tasks } from './tasks_table';

describe('<Tasks />', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    resetMockFetch();
  });

  it('warns when bot request errors', async () => {
    const errorMsg = 'Test ListBots GRPC error';
    const operation = 'list bots';
    const expectedErrorMessage = `An unexpected error occurred during ${operation}. ${errorMsg}.`;

    mockErrorListingBots(errorMsg);

    render(
      <FakeContextProvider>
        <Tasks dutId="A1234" />
      </FakeContextProvider>,
    );

    await waitFor(() =>
      expect(screen.getByText(expectedErrorMessage)).toBeVisible(),
    );
  });

  it('warns when no bot ID is found', async () => {
    mockListBots([]);

    render(
      <FakeContextProvider>
        <Tasks dutId="dut1331" />
      </FakeContextProvider>,
    );

    await waitFor(() =>
      expect(screen.getByText('Bot not found!')).toBeVisible(),
    );
  });

  it('warns when tasks request errors', async () => {
    const operation = 'list tasks';
    const errorMsg = 'Test ListTasks GRPC error';
    const expectedErrorMessage = `An unexpected error occurred during ${operation}. ${errorMsg}.`;

    mockListBots([BotInfo.fromPartial({ botId: 'bot-1' })]);
    mockErrorListingBotTasks(errorMsg);

    render(
      <FakeContextProvider>
        <Tasks dutId="A1234" />
      </FakeContextProvider>,
    );

    await waitFor(() =>
      expect(screen.getByText(expectedErrorMessage)).toBeVisible(),
    );
  });

  it('renders tasks list', async () => {
    mockListBots([BotInfo.fromPartial({ botId: 'bot-1' })]);
    mockListBotTasks(
      [
        TaskResultResponse.fromPartial({ taskId: '1', name: 'task-1' }),
        TaskResultResponse.fromPartial({ taskId: '2', name: 'task-2' }),
      ],
      '',
    );

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <Tasks dutId="A1234" />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('task-1')).toBeVisible();
      expect(screen.getByText('task-2')).toBeVisible();
      // Check pagination
      expect(screen.getByText('1-2 of 2')).toBeVisible();
    });
  });

  it('informs when no tasks found', async () => {
    const dutId = 'A1234';
    const botId = 'bot-1';
    mockListBots([BotInfo.fromPartial({ botId })]);
    mockListBotTasks([], '');

    render(
      <FakeContextProvider>
        <Tasks dutId={dutId} />
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('No tasks found')).toBeVisible();
      expect(screen.getByText(dutId)).toBeVisible();
      expect(screen.getByText(botId)).toBeVisible();
    });
  });

  it('uses provided botId directly', async () => {
    const botId = 'direct-bot-id';
    mockListBotTasks(
      [TaskResultResponse.fromPartial({ taskId: '1', name: 'direct-task' })],
      '',
    );

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <Tasks botId={botId} />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('direct-task')).toBeVisible();
    });
  });

  describe('Unified Task History for pilot devices', () => {
    it('renders unified task history for pilot device (brya)', async () => {
      mockListTaskHistory([
        TaskHistoryItem.fromPartial({
          taskId: 'sw-1',
          name: 'cros_test_platform',
          state: 'COMPLETED',
          source: TaskSource.TASK_SOURCE_SWARMING,
          taskUrl: 'https://chromeos-swarming.appspot.com/task?id=sw-1',
        }),
        TaskHistoryItem.fromPartial({
          taskId: 'mh-1',
          name: 'MobileHarness_Test',
          state: 'RUNNING',
          source: TaskSource.TASK_SOURCE_MOBILE_HARNESS,
          taskUrl:
            'https://mobileharness-fe.corp.google.com/testdetailview/job-1/mh-1',
        }),
      ]);

      render(
        <FakeContextProvider>
          <SettingsProvider>
            <Tasks dutId="chromeos8-row6-rack10-host33" board="brya" />
          </SettingsProvider>
        </FakeContextProvider>,
      );

      await waitFor(() => {
        expect(screen.getByText('cros_test_platform')).toBeVisible();
        expect(screen.getByText('MobileHarness_Test')).toBeVisible();
        expect(screen.getByText('Swarming')).toBeVisible();
        expect(screen.getByText('Mobile Harness')).toBeVisible();
      });
    });

    it('warns when unified task history request errors', async () => {
      const errorMsg = 'Test ListTaskHistory GRPC error';
      mockErrorListingTaskHistory(errorMsg);

      render(
        <FakeContextProvider>
          <SettingsProvider>
            <Tasks dutId="chromeos8-row6-rack10-host33" board="rauru" />
          </SettingsProvider>
        </FakeContextProvider>,
      );

      await waitFor(() => {
        expect(
          screen.getByText(
            `An unexpected error occurred during list task history. ${errorMsg}.`,
          ),
        ).toBeVisible();
      });
    });

    it('informs when no unified tasks found', async () => {
      const dutId = 'chromeos8-row6-rack10-host33';
      mockListTaskHistory([], '');

      render(
        <FakeContextProvider>
          <SettingsProvider>
            <Tasks dutId={dutId} board="fatcat" />
          </SettingsProvider>
        </FakeContextProvider>,
      );

      await waitFor(() => {
        expect(screen.getByText('No tasks found')).toBeVisible();
        expect(screen.getByText(dutId)).toBeVisible();
      });
    });

    it('falls back to legacy Swarming for non-pilot board', async () => {
      mockListBots([BotInfo.fromPartial({ botId: 'bot-non-al' })]);
      mockListBotTasks(
        [
          TaskResultResponse.fromPartial({
            taskId: 'legacy-1',
            name: 'legacy-swarming-task',
          }),
        ],
        '',
      );

      render(
        <FakeContextProvider>
          <SettingsProvider>
            <Tasks dutId="A1234" board="volteer" />
          </SettingsProvider>
        </FakeContextProvider>,
      );

      await waitFor(() => {
        expect(screen.getByText('legacy-swarming-task')).toBeVisible();
      });
    });
  });
});
