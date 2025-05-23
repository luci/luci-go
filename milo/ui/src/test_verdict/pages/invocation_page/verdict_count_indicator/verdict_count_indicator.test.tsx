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

import { cleanup, render, screen } from '@testing-library/react';
import { act } from 'react';

import {
  QueryTestVariantsResponse,
  ResultDBClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import {
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  QUERY_TEST_VERDICT_PAGE_SIZE,
  VerdictCountIndicator,
} from './verdict_count_indicator';

describe('<VerdictCountIndicator />', () => {
  let queryTestVariantsMock: jest.SpiedFunction<
    ResultDBClientImpl['QueryTestVariants']
  >;

  beforeEach(() => {
    jest.useFakeTimers();
    queryTestVariantsMock = jest.spyOn(
      ResultDBClientImpl.prototype,
      'QueryTestVariants',
    );
  });

  afterEach(() => {
    jest.useRealTimers();
    cleanup();
    queryTestVariantsMock.mockRestore();
  });

  it('can show unexpected count', async () => {
    queryTestVariantsMock.mockResolvedValueOnce(
      QueryTestVariantsResponse.fromPartial({
        testVariants: [
          ...Array(55).fill({
            statusV2: TestVerdict_Status.FAILED,
            statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
          }),
          ...Array(10).fill({
            statusV2: TestVerdict_Status.FLAKY,
            statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
          }),
          ...Array(5).fill({
            statusV2: TestVerdict_Status.FAILED,
            statusOverride: TestVerdict_StatusOverride.EXONERATED,
          }),
        ],
      }),
    );
    render(
      <FakeContextProvider>
        <VerdictCountIndicator invName="invocations/inv-id" />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    const indicator = screen.getByTestId('verdict-count-indicator');
    expect(indicator).toHaveTextContent('55');
    expect(indicator.title).toContain('55');
    expect(indicator.title).toContain('failed');
  });

  it('can show flaky count', async () => {
    queryTestVariantsMock.mockResolvedValueOnce(
      QueryTestVariantsResponse.fromPartial({
        testVariants: [
          ...Array(10).fill({
            statusV2: TestVerdict_Status.FLAKY,
            statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
          }),
          ...Array(5).fill({
            statusV2: TestVerdict_Status.FAILED,
            statusOverride: TestVerdict_StatusOverride.EXONERATED,
          }),
        ],
      }),
    );
    render(
      <FakeContextProvider>
        <VerdictCountIndicator invName="invocations/inv-id" />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    const indicator = screen.getByTestId('verdict-count-indicator');
    expect(indicator).toHaveTextContent('10');
    expect(indicator.title).toContain('10');
    expect(indicator.title).toContain('flaky');
  });

  it('show status icon when no verdict worse than exonerated', async () => {
    queryTestVariantsMock.mockResolvedValueOnce(
      QueryTestVariantsResponse.fromPartial({
        testVariants: [
          ...Array(5).fill({
            statusV2: TestVerdict_Status.FAILED,
            statusOverride: TestVerdict_StatusOverride.EXONERATED,
          }),
        ],
      }),
    );
    render(
      <FakeContextProvider>
        <VerdictCountIndicator invName="invocations/inv-id" />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    expect(
      screen.queryByTestId('verdict-count-indicator'),
    ).not.toBeInTheDocument();
    expect(screen.getByText('remove_circle')).toBeInTheDocument();
  });

  it("show no count when there's no verdict", async () => {
    queryTestVariantsMock.mockResolvedValueOnce(
      QueryTestVariantsResponse.fromPartial({
        testVariants: [],
      }),
    );
    render(
      <FakeContextProvider>
        <div data-testid="no-error" />
        <VerdictCountIndicator invName="invocations/inv-id" />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    expect(
      screen.queryByTestId('verdict-count-indicator'),
    ).not.toBeInTheDocument();
    expect(screen.queryByText('remove_circle')).not.toBeInTheDocument();
    expect(screen.getByTestId('no-error')).toBeInTheDocument();
  });

  it('show + sign when there are too many verdicts', async () => {
    queryTestVariantsMock.mockResolvedValueOnce(
      QueryTestVariantsResponse.fromPartial({
        testVariants: [
          ...Array(200).fill({
            statusV2: TestVerdict_Status.FAILED,
            statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
          }),
          ...Array(5).fill({
            statusV2: TestVerdict_Status.FAILED,
            statusOverride: TestVerdict_StatusOverride.EXONERATED,
          }),
        ],
      }),
    );
    render(
      <FakeContextProvider>
        <VerdictCountIndicator invName="invocations/inv-id" />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    const indicator = screen.getByTestId('verdict-count-indicator');
    expect(indicator).toHaveTextContent('99+');
    expect(indicator.title).toContain('200');
    expect(indicator.title).not.toContain('+');
  });

  it('can tell verdicts of the same statuses are all loaded when the first page is not full', async () => {
    queryTestVariantsMock.mockResolvedValueOnce(
      QueryTestVariantsResponse.fromPartial({
        testVariants: [
          ...Array(10).fill({
            statusV2: TestVerdict_Status.FAILED,
            statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
          }),
        ],
        nextPageToken: 'page2',
      }),
    );
    render(
      <FakeContextProvider>
        <VerdictCountIndicator invName="invocations/inv-id" />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    const indicator = screen.getByTestId('verdict-count-indicator');
    expect(indicator).toHaveTextContent('10');
    expect(indicator.title).toContain('10');
    expect(indicator.title).not.toContain('+');
  });

  it('can tell there might be more verdicts of the same status when the first page is full', async () => {
    queryTestVariantsMock.mockResolvedValueOnce(
      QueryTestVariantsResponse.fromPartial({
        testVariants: [
          ...Array(QUERY_TEST_VERDICT_PAGE_SIZE).fill({
            statusV2: TestVerdict_Status.FAILED,
            statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
          }),
        ],
        nextPageToken: 'page2',
      }),
    );
    render(
      <FakeContextProvider>
        <VerdictCountIndicator invName="invocations/inv-id" />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    const indicator = screen.getByTestId('verdict-count-indicator');
    expect(indicator).toHaveTextContent('99+');
    expect(indicator.title).toContain(`${QUERY_TEST_VERDICT_PAGE_SIZE}+`);
  });
});
