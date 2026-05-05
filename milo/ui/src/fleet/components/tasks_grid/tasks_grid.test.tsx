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

import { useQuery, UseQueryResult } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MRT_Row, MRT_TableInstance } from 'material-react-table';
import { act } from 'react';

import { OutputBuild } from '@/build/types';
import { usePagerContext } from '@/common/components/params_pager';
import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { mockVirtualizedListDomProperties } from '@/fleet/testing_tools/dom_mocks';
import {
  TaskResultResponse,
  TaskState,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TasksGrid, TaskGridColumnKey, TasksDetailPanel } from './tasks_grid';

jest.mock('@tanstack/react-query', () => ({
  ...jest.requireActual('@tanstack/react-query'),
  useQuery: jest.fn(),
}));

jest.mock('@/generic_libs/components/code_mirror_editor', () => ({
  CodeMirrorEditor: ({ value }: { value: string }) => <pre>{value}</pre>,
}));

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
}));

const MOCK_TASKS: TaskResultResponse[] = [
  TaskResultResponse.fromPartial({
    taskId: 'task-1',
    name: 'Task 1',
    state: TaskState.COMPLETED,
    startedTs: '2026-04-20T14:00:00Z',
    duration: 60,
    tags: ['build:123', 'dut-name:dut-1', 'buildbucket_build_id:123'],
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
    mockTrackEvent.mockClear();
    jest.useFakeTimers();
    cleanupDomMocks = mockVirtualizedListDomProperties();
    jest.mocked(useQuery).mockReturnValue({
      data: null,
      isLoading: false,
      isError: false,
      error: null,
    } as unknown as UseQueryResult<OutputBuild, Error>);
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

  it('should expand row and show detail panel on click', async () => {
    jest.mocked(useQuery).mockReturnValue({
      data: {
        output: {
          properties: {
            key1: 'value1',
          },
        },
      },
      isLoading: false,
      isError: false,
    } as unknown as UseQueryResult<OutputBuild, Error>);

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

    const expandButtons = screen.getAllByRole('button', { name: /expand/i });
    expect(expandButtons.length).toBeGreaterThan(0);

    act(() => {
      fireEvent.click(expandButtons[0]);
    });

    await act(() => jest.runAllTimersAsync());

    expect(await screen.findByText(/"key1": "value1"/)).toBeInTheDocument();
  });

  describe('TasksDetailPanel', () => {
    const mockRow = {
      original: {
        id: 'task-1',
      },
    } as unknown as MRT_Row<Record<string, unknown>>;

    const mockTable = {
      options: {
        meta: {
          taskMap: new Map([
            [
              'task-1',
              TaskResultResponse.fromPartial({
                taskId: 'task-1',
                tags: ['buildbucket_build_id:123'],
              }),
            ],
            [
              'task-3',
              TaskResultResponse.fromPartial({
                taskId: 'task-3',
                tags: ['dut-name:dut-3'],
              }),
            ],
            [
              'task-4',
              TaskResultResponse.fromPartial({
                taskId: 'task-4',
                tags: [
                  'project:chromeos',
                  'bucket:labpack_runner',
                  'buildername:labpack-repair',
                  'buildnumber:456',
                ],
              }),
            ],
          ]),
          swarmingHost: 'swarming.example.com',
          miloHost: 'milo.example.com',
        },
      },
    } as unknown as MRT_TableInstance<Record<string, unknown>>;

    it('should render loading state', async () => {
      jest.mocked(useQuery).mockReturnValue({
        isLoading: true,
      } as unknown as UseQueryResult<OutputBuild, Error>);

      render(
        <FakeContextProvider>
          <TasksDetailPanel row={mockRow} table={mockTable} />
        </FakeContextProvider>,
      );

      expect(await screen.findByRole('progressbar')).toBeInTheDocument();
    });

    it('should render error state', async () => {
      jest.mocked(useQuery).mockReturnValue({
        isError: true,
        error: new Error('Failed to fetch build'),
        isLoading: false,
      } as unknown as UseQueryResult<OutputBuild, Error>);

      render(
        <FakeContextProvider>
          <TasksDetailPanel row={mockRow} table={mockTable} />
        </FakeContextProvider>,
      );

      expect(
        (await screen.findAllByText(/Failed to fetch build/))[0],
      ).toBeInTheDocument();
    });

    it('should render success state with properties', async () => {
      jest.mocked(useQuery).mockReturnValue({
        data: {
          output: {
            properties: {
              key1: 'value1',
            },
          },
        },
        isLoading: false,
        isError: false,
      } as unknown as UseQueryResult<OutputBuild, Error>);

      render(
        <FakeContextProvider>
          <TasksDetailPanel row={mockRow} table={mockTable} />
        </FakeContextProvider>,
      );

      expect(
        (await screen.findAllByText(/"key1": "value1"/))[0],
      ).toBeInTheDocument();

      expect(useQuery).toHaveBeenCalledWith(
        expect.objectContaining({
          enabled: true,
          queryKey: expect.arrayContaining([
            expect.objectContaining({
              id: '123',
            }),
          ]),
        }),
      );
    });

    it('should render success state without properties', async () => {
      jest.mocked(useQuery).mockReturnValue({
        data: {
          output: {},
        },
        isLoading: false,
        isError: false,
      } as unknown as UseQueryResult<OutputBuild, Error>);

      render(
        <FakeContextProvider>
          <TasksDetailPanel row={mockRow} table={mockTable} />
        </FakeContextProvider>,
      );

      expect(
        (
          await screen.findAllByText(
            'No output properties found for this build.',
          )
        )[0],
      ).toBeInTheDocument();
    });

    it('should render warning when build ID is not found', async () => {
      const mockRowWithoutBuild = {
        original: {
          id: 'task-3',
        },
      } as unknown as MRT_Row<Record<string, unknown>>;

      render(
        <FakeContextProvider>
          <TasksDetailPanel row={mockRowWithoutBuild} table={mockTable} />
        </FakeContextProvider>,
      );

      expect(
        (await screen.findAllByText('Build not found!'))[0],
      ).toBeInTheDocument();
    });

    it('should fetch build properties via builder and buildNumber fallback tags when buildbucket_build_id is missing', async () => {
      const mockRowWithFallback = {
        original: {
          id: 'task-4',
        },
      } as unknown as MRT_Row<Record<string, unknown>>;

      jest.mocked(useQuery).mockReturnValue({
        data: {
          output: {
            properties: {
              fallbackKey: 'fallbackValue',
            },
          },
        },
        isLoading: false,
        isError: false,
      } as unknown as UseQueryResult<OutputBuild, Error>);

      render(
        <FakeContextProvider>
          <TasksDetailPanel row={mockRowWithFallback} table={mockTable} />
        </FakeContextProvider>,
      );

      expect(
        (await screen.findAllByText(/"fallbackKey": "fallbackValue"/))[0],
      ).toBeInTheDocument();

      expect(useQuery).toHaveBeenCalledWith(
        expect.objectContaining({
          enabled: true,
          queryKey: expect.arrayContaining([
            expect.objectContaining({
              id: '0',
              builder: {
                project: 'chromeos',
                bucket: 'labpack_runner',
                builder: 'labpack-repair',
              },
              buildNumber: 456,
            }),
          ]),
        }),
      );
    });

    it('should track event on open/mount', async () => {
      jest.mocked(useQuery).mockReturnValue({
        data: {
          output: {
            properties: {
              key1: 'value1',
            },
          },
        },
        isLoading: false,
        isError: false,
      } as unknown as UseQueryResult<OutputBuild, Error>);

      render(
        <FakeContextProvider>
          <TasksDetailPanel row={mockRow} table={mockTable} />
        </FakeContextProvider>,
      );

      expect(mockTrackEvent).toHaveBeenCalledWith(
        'task_detail_panel_opened',
        expect.objectContaining({
          componentName: 'TasksDetailPanel',
        }),
      );
    });

    it('should track event on open/mount with project if available', async () => {
      const mockRowWithProject = {
        original: {
          id: 'task-4',
        },
      } as unknown as MRT_Row<Record<string, unknown>>;

      jest.mocked(useQuery).mockReturnValue({
        data: {
          output: {
            properties: {
              fallbackKey: 'fallbackValue',
            },
          },
        },
        isLoading: false,
        isError: false,
      } as unknown as UseQueryResult<OutputBuild, Error>);

      render(
        <FakeContextProvider>
          <TasksDetailPanel row={mockRowWithProject} table={mockTable} />
        </FakeContextProvider>,
      );

      expect(mockTrackEvent).toHaveBeenCalledWith('task_detail_panel_opened', {
        componentName: 'TasksDetailPanel',
        project: 'chromeos',
      });
    });
  });
});
