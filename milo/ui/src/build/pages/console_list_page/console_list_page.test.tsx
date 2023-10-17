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

import { act, render } from '@testing-library/react';

import {
  ConsoleSnapshot as ConsoleSnapshotData,
  MiloInternal,
  QueryConsoleSnapshotsResponse,
} from '@/common/services/milo_internal';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ConsoleListPage } from './console_list_page';
import { ConsoleSnapshot } from './console_snapshot';

jest.mock('./console_snapshot', () => {
  return self.createSelectiveSpiesFromModule<
    typeof import('./console_snapshot')
  >('./console_snapshot', ['ConsoleSnapshot']);
});

function makeFakeConsoleSnapshot(id: string): ConsoleSnapshotData {
  return {
    console: {
      id,
      name: id,
      realm: 'project@root',
      repoUrl: 'https://repo.url',
      builders: [
        {
          id: {
            project: 'project',
            bucket: 'bucket',
            builder: 'builder',
          },
        },
      ],
    },
    builderSnapshots: [
      {
        builder: {
          project: 'project',
          bucket: 'bucket',
          builder: 'builder',
        },
      },
    ],
  };
}

const mockSnapshots: { [key: string]: QueryConsoleSnapshotsResponse } = {
  '': {
    snapshots: [
      makeFakeConsoleSnapshot('console-1'),
      makeFakeConsoleSnapshot('console-2'),
      makeFakeConsoleSnapshot('console-3'),
    ],
    nextPageToken: 'page2',
  },
  page2: {
    snapshots: [
      makeFakeConsoleSnapshot('console-4'),
      makeFakeConsoleSnapshot('console-5'),
      makeFakeConsoleSnapshot('console-6'),
    ],
    nextPageToken: 'page3',
  },
  page3: {
    snapshots: [
      makeFakeConsoleSnapshot('console-7'),
      makeFakeConsoleSnapshot('console-8'),
    ],
  },
};

describe('ConsoleListPage', () => {
  let consoleSnapshotSpy: jest.MockedFunctionDeep<typeof ConsoleSnapshot>;

  beforeEach(() => {
    jest.useFakeTimers();
    consoleSnapshotSpy = jest.mocked(ConsoleSnapshot);
    jest
      .spyOn(MiloInternal.prototype, 'queryConsoleSnapshots')
      .mockImplementation(async (req) => {
        const pageToken = req.pageToken || '';
        return mockSnapshots[pageToken];
      });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('e2e', async () => {
    render(
      <FakeContextProvider
        mountedPath="/p/:project"
        routerOptions={{ initialEntries: ['/p/the_project'] }}
      >
        <ConsoleListPage />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());
    await act(() => jest.runAllTimersAsync());
    await act(() => jest.runAllTimersAsync());

    expect(consoleSnapshotSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        snapshot: makeFakeConsoleSnapshot('console-1'),
      }),
      expect.anything(),
    );
    expect(consoleSnapshotSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        snapshot: makeFakeConsoleSnapshot('console-2'),
      }),
      expect.anything(),
    );
    expect(consoleSnapshotSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        snapshot: makeFakeConsoleSnapshot('console-3'),
      }),
      expect.anything(),
    );
    expect(consoleSnapshotSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        snapshot: makeFakeConsoleSnapshot('console-4'),
      }),
      expect.anything(),
    );
    expect(consoleSnapshotSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        snapshot: makeFakeConsoleSnapshot('console-5'),
      }),
      expect.anything(),
    );
    expect(consoleSnapshotSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        snapshot: makeFakeConsoleSnapshot('console-6'),
      }),
      expect.anything(),
    );
    expect(consoleSnapshotSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        snapshot: makeFakeConsoleSnapshot('console-7'),
      }),
      expect.anything(),
    );
    expect(consoleSnapshotSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        snapshot: makeFakeConsoleSnapshot('console-8'),
      }),
      expect.anything(),
    );
  });
});
