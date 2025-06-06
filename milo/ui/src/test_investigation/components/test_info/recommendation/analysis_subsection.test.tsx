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

import { render, screen } from '@testing-library/react';
import { DateTime } from 'luxon';

import { AssociatedBug } from '@/common/services/luci_analysis';
import {
  Segment,
  TestVariantBranch,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import {
  InvocationProvider,
  TestVariantProvider,
} from '@/test_investigation/context';
import { FormattedCLInfo } from '@/test_investigation/utils/test_info_utils';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestInfoContext, TestInfoContextValue } from '../context/context';

import { AnalysisSubsection } from './analysis_subsection';
import { AnalysisItemContent, generateAnalysisPoints } from './analysis_utils';

jest.mock('./analysis_utils', () => ({
  ...jest.requireActual('./analysis_utils'),
  generateAnalysisPoints: jest.fn(),
}));

const MOCK_PROJECT_ID = 'test-project';
const MOCK_TEST_ID = 'test/id/some.Test';
const MOCK_VARIANT_DEF = { def: { key1: 'val1' } };
const MOCK_RAW_INVOCATION_ID = 'inv-id-123';
const NOW = DateTime.fromISO('2024-01-10T12:00:00Z');

describe('<AnalysisSubsection />', () => {
  let mockInvocation: Invocation;
  let mockTestVariant: TestVariant;
  let defaultTestInfoContextValue: TestInfoContextValue;

  beforeEach(() => {
    jest.clearAllMocks();

    mockInvocation = Invocation.fromPartial({
      name: 'invocations/test-inv-id',
      realm: `${MOCK_PROJECT_ID}:some-realm`,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
    });
    mockTestVariant = TestVariant.fromPartial({
      testId: MOCK_TEST_ID,
      variant: MOCK_VARIANT_DEF,
    });
    defaultTestInfoContextValue = {
      testVariantBranch: TestVariantBranch.fromPartial({
        segments: [{ startPosition: '100', endPosition: '200' }],
      }),
      formattedCls: [] as FormattedCLInfo[],
      associatedBugs: [] as AssociatedBug[],
      isLoadingAssociatedBugs: false,
    };

    (generateAnalysisPoints as jest.Mock).mockReturnValue([]);
  });

  const renderComponent = (
    currentTime: DateTime = NOW,
    customInvocation?: Invocation | null,
    customTestVariant?: TestVariant | null,
    customTestInfoContext?: Partial<TestInfoContextValue>,
  ) => {
    const inv =
      customInvocation === undefined ? mockInvocation : customInvocation;
    const tv =
      customTestVariant === undefined ? mockTestVariant : customTestVariant;

    const testInfoCtxVal: TestInfoContextValue | null = customTestInfoContext
      ? { ...defaultTestInfoContextValue, ...customTestInfoContext }
      : defaultTestInfoContextValue;

    let effectiveInv: Invocation = inv as Invocation;
    let effectiveTv: TestVariant = tv as TestVariant;

    if (inv === null) {
      effectiveInv = Invocation.create();
    }
    if (tv === null) {
      effectiveTv = TestVariant.create();
    }

    return render(
      <FakeContextProvider>
        <InvocationProvider
          project="test-project"
          invocation={effectiveInv}
          rawInvocationId={MOCK_RAW_INVOCATION_ID}
        >
          <TestVariantProvider testVariant={effectiveTv}>
            <TestInfoContext.Provider value={inv && tv ? testInfoCtxVal : null}>
              <AnalysisSubsection currentTimeForAgoDt={currentTime} />
            </TestInfoContext.Provider>
          </TestVariantProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );
  };

  it('should render the "Analysis" title', () => {
    renderComponent();
    expect(
      screen.getByText('Relevant analysis', {
        selector: 'div.MuiTypography-subtitle1',
      }),
    ).toBeInTheDocument();
  });

  it('should display "There are no analysis findings." when generateAnalysisPoints returns empty', () => {
    (generateAnalysisPoints as jest.Mock).mockReturnValue([]);
    renderComponent();
    expect(
      screen.getByText('There are no analysis findings.'),
    ).toBeInTheDocument();
  });

  it('should call generateAnalysisPoints with correct arguments', () => {
    const segments = [Segment.fromPartial({ startPosition: '1' })];
    const testInfoContextWithSegments = {
      ...defaultTestInfoContextValue,
      testVariantBranch: TestVariantBranch.fromPartial({ segments }),
    };
    renderComponent(
      NOW,
      mockInvocation,
      mockTestVariant,
      testInfoContextWithSegments,
    );

    expect(generateAnalysisPoints).toHaveBeenCalledWith(
      NOW,
      segments,
      mockInvocation,
      mockTestVariant,
    );
  });

  it('should render a single analysis item correctly', () => {
    const items: AnalysisItemContent[] = [
      { text: 'Single analysis point.', status: 'success' },
    ];
    (generateAnalysisPoints as jest.Mock).mockReturnValue(items);

    renderComponent();

    expect(screen.getByText('Single analysis point.')).toBeInTheDocument();
    expect(screen.getByTestId('CheckCircleOutlineIcon')).toBeInTheDocument(); // MUI icons often have data-testid
  });

  it('should render multiple analysis items correctly', () => {
    const items: AnalysisItemContent[] = [
      { text: 'First item.', status: 'warning' },
      { text: 'Second item.', status: 'error' },
    ];
    (generateAnalysisPoints as jest.Mock).mockReturnValue(items);

    renderComponent();

    expect(screen.getByText('First item.')).toBeInTheDocument();
    expect(screen.getByTestId('WarningAmberIcon')).toBeInTheDocument();
    expect(screen.getByText('Second item.')).toBeInTheDocument();
    expect(screen.getByTestId('ErrorOutlineIcon')).toBeInTheDocument();
  });

  it('should render analysis item with default icon if status is undefined', () => {
    const items: AnalysisItemContent[] = [{ text: 'Neutral info.' }]; // No status
    (generateAnalysisPoints as jest.Mock).mockReturnValue(items);

    renderComponent();
    expect(screen.getByText('Neutral info.')).toBeInTheDocument();
    expect(screen.getByTestId('InfoOutlinedIcon')).toBeInTheDocument();
  });
});
