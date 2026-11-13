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

import '@testing-library/jest-dom';
import { UseQueryResult } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';

import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { useFetchWidgetRawSamples } from '@/crystal_ball/hooks';
import {
  createMockErrorResult,
  createMockPendingResult,
  createMockQueryResult,
} from '@/crystal_ball/tests';
import {
  FetchWidgetRawSamplesRequest,
  FetchWidgetRawSamplesResponse,
  PerfChartWidget,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { ChangesSincePreviousPoint } from './changes_since_previous_point';

jest.mock('@/crystal_ball/hooks', () => ({
  ...jest.requireActual('@/crystal_ball/hooks'),
  useFetchWidgetRawSamples: jest.fn(),
}));

const mockUseFetchWidgetRawSamples = jest.mocked(useFetchWidgetRawSamples);

const CURRENT_TS = '2026-11-02T00:00:00Z';
const PREVIOUS_TS = '2026-11-01T00:00:00Z';

function buildRow(buildId: string) {
  return createMockQueryResult(
    FetchWidgetRawSamplesResponse.fromPartial({
      rows: [
        {
          values: {
            build_branch: 'git_main',
            build_id: buildId,
            build_target: 'raven-userdebug',
          },
        },
      ],
    }),
  );
}

const emptyResult = createMockQueryResult(
  FetchWidgetRawSamplesResponse.fromPartial({ rows: [] }),
);

/**
 * Mocks the hook so that requests for the current point and the previous
 * point return the provided results.
 */
function mockResults(
  current: UseQueryResult<FetchWidgetRawSamplesResponse>,
  previous: UseQueryResult<FetchWidgetRawSamplesResponse>,
) {
  mockUseFetchWidgetRawSamples.mockImplementation(
    (request: FetchWidgetRawSamplesRequest) =>
      request.selectionContext['build_creation_timestamp'] === PREVIOUS_TS
        ? previous
        : current,
  );
}

function renderComponent() {
  return render(
    <ChangesSincePreviousPoint
      dashboardName="dashboardStates/d1"
      point={{ build_creation_timestamp: CURRENT_TS, value: 1 }}
      previousPoint={{ build_creation_timestamp: PREVIOUS_TS, value: 2 }}
      seriesId="s1"
      seriesIndex={0}
      widget={PerfChartWidget.fromPartial({})}
      widgetId="w1"
    />,
  );
}

describe('ChangesSincePreviousPoint', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('links to the CLs between the previous and the selected build', () => {
    mockResults(buildRow('200'), buildRow('100'));
    renderComponent();

    expect(
      screen.getByRole('link', {
        name: COMMON_MESSAGES.VIEW_CLS_SINCE_PREVIOUS_POINT,
      }),
    ).toHaveAttribute(
      'href',
      'https://android-build.corp.google.com/range_search/cls/from_id/200/to_id/100/?includeFrom=1&includeTo=0&selectedFromTarget=raven-userdebug',
    );
    expect(
      screen.getByText('100 → 200 | git_main | raven-userdebug'),
    ).toBeInTheDocument();
  });

  it('restricts the previous build to the same branch and target', () => {
    mockResults(buildRow('200'), buildRow('100'));
    renderComponent();

    const previousRequest = mockUseFetchWidgetRawSamples.mock.calls
      .map(([request]) => request)
      .find(
        (request) =>
          request.selectionContext['build_creation_timestamp'] === PREVIOUS_TS,
      );
    expect(previousRequest?.selectionContext).toEqual({
      build_branch: 'git_main',
      build_creation_timestamp: PREVIOUS_TS,
      build_target: 'raven-userdebug',
    });
    expect(previousRequest?.orderBy).toBe('build_creation_timestamp desc');
    expect(previousRequest?.pageSize).toBe(1);
  });

  it('shows a loading state', () => {
    mockResults(createMockPendingResult(), createMockPendingResult());
    renderComponent();

    expect(
      screen.getByText(COMMON_MESSAGES.LOOKING_UP_CHANGES),
    ).toBeInTheDocument();
  });

  it('shows an error state', () => {
    mockResults(buildRow('200'), createMockErrorResult(new Error('boom')));
    renderComponent();

    expect(
      screen.getByText(COMMON_MESSAGES.FAILED_TO_FETCH_CL_RANGE),
    ).toBeInTheDocument();
  });

  it('explains that non-sequential builds have no CL range', () => {
    mockResults(buildRow('P200'), buildRow('100'));
    renderComponent();

    expect(
      screen.getByText(COMMON_MESSAGES.CL_RANGE_UNAVAILABLE),
    ).toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });

  it('shows a message when no previous build is found', () => {
    mockResults(buildRow('200'), emptyResult);
    renderComponent();

    expect(
      screen.getByText(COMMON_MESSAGES.NO_PREVIOUS_BUILD_FOUND),
    ).toBeInTheDocument();
  });

  it('renders nothing when the selected point has no samples', () => {
    mockResults(emptyResult, emptyResult);
    renderComponent();

    expect(
      screen.queryByTestId('changes-since-previous-point'),
    ).not.toBeInTheDocument();
  });
});
