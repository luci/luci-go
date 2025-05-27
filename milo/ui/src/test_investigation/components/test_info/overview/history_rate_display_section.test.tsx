// src/test_investigation/components/test_info/overview/history_rate_display_section.test.tsx
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
// limitations under the License.

import { render, screen, fireEvent } from '@testing-library/react';

import { AssociatedBug } from '@/common/services/luci_analysis';
import {
  Segment,
  Segment_Counts,
  TestVariantBranch,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { NO_HISTORY_DATA_TEXT } from '@/test_investigation/components/test_info/types';
import {
  InvocationProvider,
  TestVariantProvider,
} from '@/test_investigation/context';
import { FormattedCLInfo } from '@/test_investigation/utils/test_info_utils';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestInfoContext, TestInfoContextValue } from '../context/context';

import { HistoryRateDisplaySection } from './history_rate_display_section';

const MOCK_PROJECT_ID = 'test-project';
const MOCK_TEST_ID = 'test/id/some.Test';
const MOCK_VARIANT_DEF = { def: { key1: 'val1' } };
const MOCK_RAW_INVOCATION_ID = 'inv-id-123';

const createMockSegment = (
  _id: string,
  startPos: string,
  endPos: string,
  unexpected: number,
  total: number,
  startHour?: string,
): Segment =>
  Segment.fromPartial({
    startPosition: startPos,
    endPosition: endPos,
    counts: Segment_Counts.fromPartial({
      unexpectedResults: unexpected,
      totalResults: total,
    }),
    startHour: startHour || undefined,
    hasStartChangepoint: true,
  });

describe('<HistoryRateDisplaySection />', () => {
  let mockInvocation: Invocation;
  let mockTestVariant: TestVariant;
  let defaultTestInfoContextValue: TestInfoContextValue;

  beforeEach(() => {
    mockInvocation = Invocation.fromPartial({
      realm: `${MOCK_PROJECT_ID}:some-realm`,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
    });
    mockTestVariant = TestVariant.fromPartial({
      testId: MOCK_TEST_ID,
      variant: MOCK_VARIANT_DEF, // Pass the inner 'def' object
    });
    defaultTestInfoContextValue = {
      testVariantBranch: TestVariantBranch.fromPartial({ segments: [] }),
      formattedCls: [] as FormattedCLInfo[],
      associatedBugs: [] as AssociatedBug[],
      isLoadingAssociatedBugs: false,
    };
  });

  const renderComponent = (
    currentTimeForAgo?: Date,
    customInvocation?: Invocation,
    customTestVariant?: TestVariant,
    customTestInfoContextValue?: Partial<TestInfoContextValue>,
  ) => {
    const inv = customInvocation || mockInvocation;
    const tv = customTestVariant || mockTestVariant;
    const testInfoCtxVal: TestInfoContextValue = {
      ...defaultTestInfoContextValue,
      ...customTestInfoContextValue,
    };
    return render(
      <FakeContextProvider>
        <InvocationProvider
          invocation={inv}
          rawInvocationId={MOCK_RAW_INVOCATION_ID}
        >
          <TestVariantProvider testVariant={tv}>
            <TestInfoContext.Provider value={testInfoCtxVal}>
              <HistoryRateDisplaySection
                currentTimeForAgo={currentTimeForAgo}
              />
            </TestInfoContext.Provider>
          </TestVariantProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );
  };

  it('should display no history data text when segments are empty', () => {
    renderComponent(undefined, undefined, undefined, {
      testVariantBranch: TestVariantBranch.fromPartial({ segments: [] }),
    });
    expect(screen.getByText(NO_HISTORY_DATA_TEXT)).toBeInTheDocument();
  });

  it('should display no history data text when testVariantBranch is null', () => {
    renderComponent(undefined, undefined, undefined, {
      testVariantBranch: null,
    });
    expect(screen.getByText(NO_HISTORY_DATA_TEXT)).toBeInTheDocument();
  });

  it('should display no history data text when testVariantBranch is undefined', () => {
    renderComponent(undefined, undefined, undefined, {
      testVariantBranch: undefined,
    });
    expect(screen.getByText(NO_HISTORY_DATA_TEXT)).toBeInTheDocument();
  });

  it('should render the title', () => {
    const segments = [createMockSegment('s1', '100', '110', 10, 100)];
    renderComponent(undefined, undefined, undefined, {
      testVariantBranch: TestVariantBranch.fromPartial({ segments }),
    });
    expect(
      screen.getByText('Postsubmit history (Changepoint failure rate)'),
    ).toBeInTheDocument();
  });

  it('should display a single segment as contextual', async () => {
    const segmentsData = [
      createMockSegment('s1', '100', '110', 10, 100, '2024-01-10T10:00:00Z'),
    ];
    const currentInv = Invocation.fromPartial({
      ...mockInvocation,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
    });
    renderComponent(new Date('2024-01-10T12:00:00Z'), currentInv, undefined, {
      testVariantBranch: TestVariantBranch.fromPartial({
        segments: segmentsData,
      }),
    });
    const failureRateView = screen.getByText('10%');
    expect(failureRateView).toBeInTheDocument();
    const boxElement = failureRateView.closest('div[class*="MuiBox-root"]');
    expect(boxElement).toHaveStyle(
      'border: 2px solid var(--gm3-color-primary)',
    );
    fireEvent.mouseOver(failureRateView);
    const tooltip = await screen.findByRole('tooltip');
    expect(tooltip).toHaveTextContent(
      'Segment: 100 - 110 (started 2 hours ago) (Contextual to Invocation Commit)',
    );

    expect(
      screen.getByLabelText('This is the oldest recorded history'),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText('This is the newest recorded history'),
    ).toBeInTheDocument();
  });

  it('should display three segments with invocation in the middle', async () => {
    const segmentsData = [
      createMockSegment('sNew', '111', '120', 5, 100),
      createMockSegment('sCtx', '100', '110', 50, 100),
      createMockSegment('sOld', '90', '99', 95, 100),
    ];
    const currentInv = Invocation.fromPartial({
      ...mockInvocation,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
    });
    renderComponent(undefined, currentInv, undefined, {
      testVariantBranch: TestVariantBranch.fromPartial({
        segments: segmentsData,
      }),
    });
    expect(screen.getByText('5%')).toBeInTheDocument();
    expect(screen.getByText('50%')).toBeInTheDocument();
    expect(screen.getByText('95%')).toBeInTheDocument();
    const contextualBox = screen
      .getByText('50%')
      .closest('div[class*="MuiBox-root"]');
    expect(contextualBox).toHaveStyle(
      'border: 2px solid var(--gm3-color-primary)',
    );

    expect(
      screen.getByLabelText('This is the oldest recorded history'),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText('This is the newest recorded history'),
    ).toBeInTheDocument();
    expect(screen.getAllByTestId('ArrowForwardIcon')).toHaveLength(2);
  });

  it('should display five segments with invocation in middle, showing correct indicators', async () => {
    const segmentsData = [
      createMockSegment('sN2', '121', '130', 1, 100),
      createMockSegment('sN1', '111', '120', 2, 100),
      createMockSegment('sCtx', '100', '110', 3, 100),
      createMockSegment('sO1', '90', '99', 4, 100),
      createMockSegment('sO2', '80', '89', 5, 100),
    ];
    const currentInv = Invocation.fromPartial({
      ...mockInvocation,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
    });
    renderComponent(undefined, currentInv, undefined, {
      testVariantBranch: TestVariantBranch.fromPartial({
        segments: segmentsData,
      }),
    });
    expect(screen.getByText('2%')).toBeInTheDocument();
    expect(screen.getByText('3%')).toBeInTheDocument();
    expect(screen.getByText('4%')).toBeInTheDocument();

    expect(
      screen.getByLabelText('Older history available'),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText('More recent history available'),
    ).toBeInTheDocument();
    expect(screen.getAllByTestId('ArrowForwardIcon')).toHaveLength(4);
  });

  it('should render the "View full history" link correctly', () => {
    const segmentsData = [createMockSegment('s1', '100', '110', 10, 100)];
    renderComponent(undefined, undefined, undefined, {
      testVariantBranch: TestVariantBranch.fromPartial({
        segments: segmentsData,
      }),
    });
    const link = screen.getByRole('link', { name: /View full history/i });
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute(
      'href',
      `/ui/test/test-project/test%2Fid%2Fsome.Test?q=V%3Akey1%3Dval1`,
    );
  });

  it('should use currentTimeForAgo for formatting "ago" text', async () => {
    const fixedCurrentTime = new Date('2025-05-22T14:00:00Z');
    const segmentStartTime = '2025-05-22T10:00:00Z';
    const segmentsData = [
      createMockSegment('s1', '100', '110', 10, 100, segmentStartTime),
    ];
    const currentInv = Invocation.fromPartial({
      ...mockInvocation,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
    });
    renderComponent(fixedCurrentTime, currentInv, undefined, {
      testVariantBranch: TestVariantBranch.fromPartial({
        segments: segmentsData,
      }),
    });
    const failureRateView = screen.getByText('10%');
    fireEvent.mouseOver(failureRateView);
    const tooltip = await screen.findByRole('tooltip');
    expect(tooltip).toHaveTextContent('started 4 hours ago');
  });
});
