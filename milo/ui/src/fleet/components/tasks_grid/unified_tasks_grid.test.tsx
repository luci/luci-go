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

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { act } from 'react';

import { usePagerContext } from '@/common/components/params_pager';
import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { mockVirtualizedListDomProperties } from '@/fleet/testing_tools/dom_mocks';
import {
  TaskHistoryItem,
  TaskSource,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { UnifiedTasksGrid } from './unified_tasks_grid';

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
}));

const MOCK_TASKS: TaskHistoryItem[] = [
  TaskHistoryItem.fromPartial({
    taskId: 'sw-1',
    name: 'cros_test_platform',
    state: 'COMPLETED',
    source: TaskSource.TASK_SOURCE_SWARMING,
    buildVersion: 'R128-15964.0.0',
    startTime: '2026-09-24T20:00:00Z',
    duration: { seconds: '120', nanos: 0 },
    taskUrl: 'https://chromeos-swarming.appspot.com/task?id=sw-1',
  }),
  TaskHistoryItem.fromPartial({
    taskId: 'mh-1',
    name: 'v2/al-ate/integration/mobly/desktop_lsnexus',
    state: 'RUNNING',
    source: TaskSource.TASK_SOURCE_MOBILE_HARNESS,
    startTime: '2026-09-24T22:11:49Z',
    taskUrl:
      'https://mobileharness-fe.corp.google.com/testdetailview/job-1/mh-1',
  }),
];

function TestWrapper({
  tasks,
  nextPageToken,
}: {
  tasks: readonly TaskHistoryItem[];
  nextPageToken?: string;
}) {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [10, 25, 50, 100],
    defaultPageSize: 50,
  });

  return (
    <UnifiedTasksGrid
      tasks={tasks}
      pagerCtx={pagerCtx}
      nextPageToken={nextPageToken}
    />
  );
}

describe('UnifiedTasksGrid', () => {
  let cleanupDomMocks: () => void;

  beforeEach(() => {
    mockTrackEvent.mockClear();
    jest.useFakeTimers();
    cleanupDomMocks = mockVirtualizedListDomProperties();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
    cleanup();
    cleanupDomMocks();
  });

  it('renders source chips, task names, and deep links', async () => {
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

    expect(screen.getByText('Source')).toBeInTheDocument();
    expect(screen.getByText('Task')).toBeInTheDocument();
    expect(screen.getByText('Build version')).toBeInTheDocument();

    expect(screen.getByText('Swarming')).toBeInTheDocument();
    expect(screen.getByText('Mobile Harness')).toBeInTheDocument();

    const swLink = screen.getByRole('link', { name: 'cros_test_platform' });
    expect(swLink).toHaveAttribute(
      'href',
      'https://chromeos-swarming.appspot.com/task?id=sw-1',
    );

    const mhLink = screen.getByRole('link', {
      name: 'v2/al-ate/integration/mobly/desktop_lsnexus',
    });
    expect(mhLink).toHaveAttribute(
      'href',
      'https://mobileharness-fe.corp.google.com/testdetailview/job-1/mh-1',
    );
  });

  it('tracks analytics event when clicking task deep link', async () => {
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

    const mhLink = screen.getByRole('link', {
      name: 'v2/al-ate/integration/mobly/desktop_lsnexus',
    });
    fireEvent.click(mhLink);

    expect(mockTrackEvent).toHaveBeenCalledWith('unified_task_link_clicked', {
      componentName: 'UnifiedTasksGrid',
    });
  });

  it('displays pagination count accurately with and without nextPageToken', async () => {
    const { unmount } = render(
      <FakeContextProvider>
        <SettingsProvider>
          <ShortcutProvider>
            <TestWrapper tasks={MOCK_TASKS} nextPageToken="token-next" />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(screen.getByText('1-2 of more than 2')).toBeInTheDocument();
    unmount();

    const unknownSourceTask: TaskHistoryItem[] = [
      TaskHistoryItem.fromPartial({
        taskId: 'unk-1',
        name: 'unknown_source_task',
        state: 'COMPLETED',
        source: TaskSource.TASK_SOURCE_UNSPECIFIED,
      }),
    ];

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <ShortcutProvider>
            <TestWrapper tasks={unknownSourceTask} />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(screen.getByText('Unknown')).toBeInTheDocument();
    expect(screen.getByText('1-1 of 1')).toBeInTheDocument();
  });

  it('applies row--failure class to Mobile Harness ERROR, FAIL, and ALLOC_ERROR tasks', async () => {
    const errorTasks: TaskHistoryItem[] = [
      TaskHistoryItem.fromPartial({
        taskId: 'mh-err',
        name: 'mh_error_test',
        state: 'ERROR',
        source: TaskSource.TASK_SOURCE_MOBILE_HARNESS,
      }),
      TaskHistoryItem.fromPartial({
        taskId: 'mh-fail',
        name: 'mh_fail_test',
        state: 'FAIL',
        source: TaskSource.TASK_SOURCE_MOBILE_HARNESS,
      }),
      TaskHistoryItem.fromPartial({
        taskId: 'mh-alloc-err',
        name: 'mh_alloc_error_test',
        state: 'ALLOC_ERROR',
        source: TaskSource.TASK_SOURCE_MOBILE_HARNESS,
      }),
    ];

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <ShortcutProvider>
            <TestWrapper tasks={errorTasks} />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const errRow = screen.getByText('mh_error_test').closest('tr');
    const failRow = screen.getByText('mh_fail_test').closest('tr');
    const allocErrRow = screen.getByText('mh_alloc_error_test').closest('tr');
    expect(errRow).toHaveClass('row--failure');
    expect(failRow).toHaveClass('row--failure');
    expect(allocErrRow).toHaveClass('row--failure');
  });
});
