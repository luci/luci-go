// Copyright 2024 The LUCI Authors.
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

import { NEVER_PROMISE } from '@/common/constants/utils';
import {
  ChangepointsClientImpl,
  QueryChangepointGroupSummariesResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { URLObserver } from '@/testing_tools/url_observer';

import { RecentRegressions } from './recent_regressions';

const mockedResponse = QueryChangepointGroupSummariesResponse.fromJSON({
  groupSummaries: [
    {
      canonicalChangepoint: {
        project: 'proj',
        testId: 'testid',
        variantHash: 'vhash',
        refHash: 'refhash',
        ref: {
          gitiles: {
            host: 'chromium.googlesource.com',
            project: 'chromium/src',
            ref: 'refs/heads/main',
          },
        },
        startHour: '2020-01-01',
        startPositionLowerBound99th: '123',
        startPositionUpperBound99th: '125',
        nominalStartPosition: '124',
        previousSegmentNominalEndPosition: '120',
      },
      statistics: {
        count: 1,
        unexpectedVerdictRateBefore: {
          average: 0.012269938,
          buckets: {
            countLess5Percent: 1,
          },
        },
        unexpectedVerdictRateAfter: {
          average: 0.0625,
          buckets: {
            countAbove5LessThan95Percent: 1,
          },
        },
        unexpectedVerdictRateCurrent: {
          average: 0.0625,
          buckets: {
            countAbove5LessThan95Percent: 1,
          },
        },
        unexpectedVerdictRateChange: {
          countIncreased0To20Percent: 1,
        },
      },
    },
  ],
});

describe('<RecentRegressions />', () => {
  let queryGroupSummariesMock: jest.SpiedFunction<
    ChangepointsClientImpl['QueryGroupSummaries']
  >;

  beforeEach(() => {
    jest.useFakeTimers();
    queryGroupSummariesMock = jest
      .spyOn(ChangepointsClientImpl.prototype, 'QueryGroupSummaries')
      .mockResolvedValue(NEVER_PROMISE);
  });

  afterEach(() => {
    cleanup();
    queryGroupSummariesMock.mockClear();
    jest.useRealTimers();
  });

  it('can set predicate via search param', () => {
    render(
      <FakeContextProvider
        mountedPath="/"
        routerOptions={{
          initialEntries: ['/?cp=%7B"testIdContain"%3A"contain"%7D'],
        }}
      >
        <RecentRegressions project="proj" />
      </FakeContextProvider>,
    );
    const containInputEle = screen.getByLabelText('Test ID contain');
    expect(containInputEle).toHaveValue('contain');
  });

  it('can save predicate to search param', () => {
    const urlCallback = jest.fn();
    render(
      <FakeContextProvider mountedPath="/">
        <RecentRegressions project="proj" />
        <URLObserver callback={urlCallback} />
      </FakeContextProvider>,
    );

    const containInputEle = screen.getByLabelText('Test ID contain');
    fireEvent.change(containInputEle, { target: { value: 'contain' } });
    fireEvent.click(screen.getByText('Apply Filter'));

    expect(urlCallback).toHaveBeenLastCalledWith(
      expect.objectContaining({
        search: {
          cp: JSON.stringify({ testIdContain: 'contain' }),
        },
      }),
    );
  });

  it('can pass predicate to regression details link', async () => {
    queryGroupSummariesMock.mockResolvedValue(mockedResponse);
    render(
      <FakeContextProvider
        mountedPath="test"
        routerOptions={{
          initialEntries: ['/test?cp=%7B"testIdContain"%3A"contain"%7D'],
        }}
      >
        <RecentRegressions project="proj" />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());
    const link = screen.getByText('details').getAttribute('href')!;
    const url = new URL(link, 'http://placeholder.com');
    const searchParams = Object.fromEntries(url.searchParams.entries());
    expect(searchParams).toEqual({
      cp: JSON.stringify({ testIdContain: 'contain' }),
      nsp: '124',
      sh: '2020-01-01',
      tvb: 'projects/proj/tests/testid/variants/vhash/refs/refhash',
    });
  });
});
