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

import { render, screen } from '@testing-library/react';
import React from 'react';

import { useAuthState } from '@/common/components/auth_state_provider';
import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { useTasks } from '@/fleet/hooks/swarming_hooks';
import {
  StateQuery,
  TaskResultResponse,
  TaskState,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TasksTable } from './tasks_table';

jest.mock('@/common/components/auth_state_provider');
jest.mock('@/fleet/hooks/swarming_hooks');
jest.mock('@/swarming/hooks/prpc_clients', () => ({
  useTasksClient: jest.fn(),
}));

function TestWrapper({ children }: { children: React.ReactNode }) {
  return (
    <FakeContextProvider>
      <SettingsProvider>
        <ShortcutProvider>{children}</ShortcutProvider>
      </SettingsProvider>
    </FakeContextProvider>
  );
}

describe('TasksTable', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (useAuthState as jest.Mock).mockReturnValue({
      email: 'user@google.com',
    });
  });

  it('renders error message if not logged in', () => {
    (useAuthState as jest.Mock).mockReturnValue({ email: undefined });
    render(
      <TestWrapper>
        <TasksTable />
      </TestWrapper>,
    );

    expect(screen.getByText('Not logged in.')).toBeInTheDocument();
  });

  it('renders empty alert when no tasks returned', () => {
    (useTasks as jest.Mock).mockReturnValue({
      tasks: [],
      isLoading: false,
      isError: false,
    });

    render(
      <TestWrapper>
        <TasksTable />
      </TestWrapper>,
    );

    expect(
      screen.getByText('No active tasks currently running or pending.'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('No completed task history found.'),
    ).toBeInTheDocument();
  });

  it('partitions into Active Tasks and Task History when active tasks exist', () => {
    const mockActiveTask = TaskResultResponse.fromPartial({
      taskId: 'task-1',
      name: 'Running Task',
      state: TaskState.RUNNING,
    });
    const mockHistoryTask = TaskResultResponse.fromPartial({
      taskId: 'task-2',
      name: 'Completed Task',
      state: TaskState.COMPLETED,
    });

    (useTasks as jest.Mock).mockImplementation(({ state }) => {
      if (state === StateQuery.QUERY_PENDING_RUNNING) {
        return { tasks: [mockActiveTask], isLoading: false, isError: false };
      }
      return { tasks: [mockHistoryTask], isLoading: false, isError: false };
    });

    render(
      <TestWrapper>
        <TasksTable />
      </TestWrapper>,
    );

    expect(screen.getByText('Active Tasks')).toBeInTheDocument();
    expect(screen.getByText('Task History')).toBeInTheDocument();
    expect(screen.getByText('Running Task')).toBeInTheDocument();
    expect(screen.getByText('Completed Task')).toBeInTheDocument();
  });

  it('renders independent section errors when history query fails but active query succeeds', () => {
    const mockActiveTask = TaskResultResponse.fromPartial({
      taskId: 'task-1',
      name: 'Running Task',
      state: TaskState.RUNNING,
    });

    (useTasks as jest.Mock).mockImplementation(({ state }) => {
      if (state === StateQuery.QUERY_PENDING_RUNNING) {
        return { tasks: [mockActiveTask], isLoading: false, isError: false };
      }
      return {
        tasks: undefined,
        isLoading: false,
        isError: true,
        error: new Error('Network error'),
      };
    });

    render(
      <TestWrapper>
        <TasksTable />
      </TestWrapper>,
    );

    expect(screen.getByText('Running Task')).toBeInTheDocument();
    expect(
      screen.getByText('An unexpected error occurred: Network error'),
    ).toBeInTheDocument();
  });

  it('renders Task History grid when history tasks is empty but nextPageToken exists', () => {
    (useTasks as jest.Mock).mockImplementation(({ state }) => {
      if (state === StateQuery.QUERY_PENDING_RUNNING) {
        return { tasks: [], isLoading: false, isError: false };
      }
      return {
        tasks: [],
        nextPageToken: 'token-page-2',
        isLoading: false,
        isError: false,
      };
    });

    render(
      <TestWrapper>
        <TasksTable />
      </TestWrapper>,
    );

    expect(screen.getByText('Active Tasks')).toBeInTheDocument();
    expect(screen.getByText('Task History')).toBeInTheDocument();
    expect(
      screen.queryByText('No completed task history found.'),
    ).not.toBeInTheDocument();
  });
});
