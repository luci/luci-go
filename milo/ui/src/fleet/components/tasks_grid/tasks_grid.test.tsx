// Copyright 2026 The LUCI Authors.
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

import { cleanup, render, screen } from '@testing-library/react';
import { act } from 'react';

import { usePagerContext } from '@/common/components/params_pager';
import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { mockVirtualizedListDomProperties } from '@/fleet/testing_tools/dom_mocks';
import {
  TaskResultResponse,
  TaskState,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TasksGrid, TaskGridColumnKey } from './tasks_grid';

const MOCK_TASKS: TaskResultResponse[] = [
  TaskResultResponse.fromPartial({
    taskId: 'task-1',
    name: 'Task 1',
    state: TaskState.COMPLETED,
    startedTs: '2026-04-20T14:00:00Z',
    duration: 60,
    tags: ['build:123', 'dut-name:dut-1'],
  }),
  TaskResultResponse.fromPartial({
    taskId: 'task-2',
    name: 'Task 2',
    state: TaskState.RUNNING,
    startedTs: '2026-04-20T14:05:00Z',
    tags: ['build:124', 'dut-name:dut-2'],
  }),
];

const COLUMN_KEYS: TaskGridColumnKey[] = [
  'task',
  'buildVersion',
  'result',
  'started',
  'duration',
  'dut_name',
];

function TestWrapper({
  tasks,
  nextPageToken,
}: {
  tasks: TaskResultResponse[];
  nextPageToken?: string;
}) {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [10, 25, 50, 100],
    defaultPageSize: 10,
  });

  return (
    <TasksGrid
      tasks={tasks}
      pagerCtx={pagerCtx}
      columnKeys={COLUMN_KEYS}
      nextPageToken={nextPageToken}
      swarmingHost="swarming.example.com"
      miloHost="milo.example.com"
    />
  );
}

describe('TasksGrid', () => {
  let cleanupDomMocks: () => void;

  beforeEach(() => {
    jest.useFakeTimers();
    cleanupDomMocks = mockVirtualizedListDomProperties();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
    cleanup();
    cleanupDomMocks();
  });

  it('should render columns and data', async () => {
    render(
      <FakeContextProvider>
        <SettingsProvider>
          <ShortcutProvider>
            <TestWrapper tasks={MOCK_TASKS} />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    // Check headers
    expect(screen.getByText('Task')).toBeInTheDocument();
    expect(screen.getByText('Build version')).toBeInTheDocument();
    expect(screen.getByText('Result')).toBeInTheDocument();

    // Check data
    expect(screen.getByText('Task 1')).toBeInTheDocument();
    expect(screen.getByText('Task 2')).toBeInTheDocument();
    expect(screen.getByText('123')).toBeInTheDocument(); // build version
    expect(screen.getByText('dut-1')).toBeInTheDocument(); // dut name
  });

  it('should render pagination with correct text', async () => {
    render(
      <FakeContextProvider>
        <SettingsProvider>
          <ShortcutProvider>
            <TestWrapper tasks={MOCK_TASKS} nextPageToken="token-2" />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    // Check pagination text
    expect(screen.getByText('1-2 of more than 2')).toBeInTheDocument();
  });
});
