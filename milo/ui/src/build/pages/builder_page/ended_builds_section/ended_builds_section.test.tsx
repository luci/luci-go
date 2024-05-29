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

import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { DateTime } from 'luxon';
import { act } from 'react';

import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import {
  BuildsClientImpl,
  SearchBuildsResponse,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { EndedBuildTable } from './ended_build_table';
import { EndedBuildsSection } from './ended_builds_section';

jest.mock('./ended_build_table', () => ({
  EndedBuildTable: jest.fn(() => <></>),
}));

const builderId = {
  bucket: 'buck',
  builder: 'builder',
  project: 'proj',
};

function createMockBuild(id: string): Build {
  return Build.fromPartial({
    id,
    builder: builderId,
    status: Status.SUCCESS,
    createTime: '2020-01-01',
  });
}

const builds = [
  createMockBuild('1234'),
  createMockBuild('2345'),
  createMockBuild('3456'),
  createMockBuild('4567'),
  createMockBuild('5678'),
];

const pages: {
  [timestamp: string]: { [pageToken: string]: SearchBuildsResponse };
} = {
  '': {
    '': {
      builds: builds.slice(0, 2),
      nextPageToken: 'page2',
    },
    page2: {
      builds: builds.slice(2, 4),
      nextPageToken: 'page3',
    },
    page3: {
      builds: builds.slice(4, 5),
      nextPageToken: '',
    },
  },
  '2020-02-02T02:02:02.000+00:00': {
    '': {
      builds: builds.slice(1, 3),
      nextPageToken: 'page2',
    },
    page2: {
      builds: builds.slice(3, 5),
      nextPageToken: '',
    },
  },
};

describe('EndedBuildsSection', () => {
  let endedBuildsTableMock: jest.MockedFunctionDeep<typeof EndedBuildTable>;

  beforeEach(() => {
    jest.useFakeTimers();
    jest
      .spyOn(BuildsClientImpl.prototype, 'SearchBuilds')
      .mockImplementation(
        async ({ pageToken, predicate }) =>
          pages[predicate?.createTime?.endTime || ''][pageToken || ''],
      );
    endedBuildsTableMock = jest.mocked(EndedBuildTable);
  });

  afterEach(() => {
    jest.useRealTimers();
    cleanup();
  });

  it('should navigate between pages properly', async () => {
    render(
      <FakeContextProvider>
        <EndedBuildsSection builderId={builderId} />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());

    const prevPageLink = screen.getByText('Previous Page');
    const nextPageLink = screen.getByText('Next Page');

    expect(prevPageLink).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page2');
    expect(endedBuildsTableMock).toHaveBeenCalledWith(
      {
        endedBuilds: builds.slice(0, 2),
        isLoading: false,
      },
      expect.anything(),
    );
    endedBuildsTableMock.mockClear();

    fireEvent.click(nextPageLink);
    await act(() => jest.runAllTimersAsync());
    expect(prevPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink).toHaveAttribute('href', '/');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page3');
    expect(endedBuildsTableMock).toHaveBeenCalledWith(
      {
        endedBuilds: builds.slice(2, 4),
        isLoading: false,
      },
      expect.anything(),
    );
    endedBuildsTableMock.mockClear();

    fireEvent.click(nextPageLink);
    await act(() => jest.runAllTimersAsync());
    expect(prevPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink).toHaveAttribute('href', '/?cursor=page2');
    expect(nextPageLink).toHaveAttribute('aria-disabled', 'true');
    expect(endedBuildsTableMock).toHaveBeenCalledWith(
      {
        endedBuilds: builds.slice(4, 6),
        isLoading: false,
      },
      expect.anything(),
    );
    endedBuildsTableMock.mockClear();

    fireEvent.click(prevPageLink);
    await act(() => jest.runAllTimersAsync());
    expect(prevPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(prevPageLink).toHaveAttribute('href', '/');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page3');
    expect(endedBuildsTableMock).toHaveBeenCalledWith(
      {
        endedBuilds: builds.slice(2, 4),
        isLoading: false,
      },
      expect.anything(),
    );
    endedBuildsTableMock.mockClear();

    fireEvent.click(prevPageLink);
    await act(() => jest.runAllTimersAsync());
    expect(prevPageLink).toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(nextPageLink).toHaveAttribute('href', '/?cursor=page2');
    expect(endedBuildsTableMock).toHaveBeenCalledWith(
      {
        endedBuilds: builds.slice(0, 2),
        isLoading: false,
      },
      expect.anything(),
    );
    endedBuildsTableMock.mockClear();
  });

  it('should clear page tokens after date filter is reset', async () => {
    render(
      <FakeContextProvider>
        <EndedBuildsSection builderId={builderId} />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());

    expect(endedBuildsTableMock).toHaveBeenCalledWith(
      {
        endedBuilds: builds.slice(0, 2),
        isLoading: false,
      },
      expect.anything(),
    );
    endedBuildsTableMock.mockClear();

    const prevPageLink = screen.getByText('Previous Page');
    const nextPageLink = screen.getByText('Next Page');

    fireEvent.click(nextPageLink);
    await act(() => jest.runAllTimersAsync());
    expect(prevPageLink).not.toHaveAttribute('aria-disabled', 'true');
    expect(endedBuildsTableMock).toHaveBeenCalledWith(
      {
        endedBuilds: builds.slice(2, 4),
        isLoading: false,
      },
      expect.anything(),
    );
    endedBuildsTableMock.mockClear();

    jest.setSystemTime(
      DateTime.fromISO('2020-02-02T02:02:02.000+00:00').toMillis(),
    );
    fireEvent.click(screen.getByTestId('CalendarIcon'));
    fireEvent.click(screen.getByText('Today'));

    await act(() => jest.runAllTimersAsync());
    // Prev page tokens are purged.
    expect(prevPageLink).toHaveAttribute('aria-disabled', 'true');
    expect(endedBuildsTableMock).toHaveBeenCalledWith(
      {
        endedBuilds: builds.slice(1, 3),
        isLoading: false,
      },
      expect.anything(),
    );
  });
});
