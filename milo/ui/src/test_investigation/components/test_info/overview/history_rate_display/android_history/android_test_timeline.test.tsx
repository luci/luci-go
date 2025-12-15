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

import { useListBuilds } from '@/common/hooks/gapi_query/android_build/android_build';
import { useGetTestResultFluxgateSegmentSummaries } from '@/common/hooks/gapi_query/android_fluxgate/android_fluxgate';
import { OutputTestVerdict } from '@/common/types/verdict';
import { RootInvocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/root_invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import {
  InvocationProvider,
  TestVariantProvider,
} from '@/test_investigation/context';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AndroidTestTimeline } from './android_test_timeline';

// Mock the GAPI hooks
jest.mock('@/common/hooks/gapi_query/android_build/android_build');
jest.mock('@/common/hooks/gapi_query/android_fluxgate/android_fluxgate');

const mockUseListBuilds = useListBuilds as jest.Mock;
const mockUseGetTestResultFluxgateSegmentSummaries =
  useGetTestResultFluxgateSegmentSummaries as jest.Mock;

const MOCK_PROJECT_ID = 'test-project';
const MOCK_TEST_ID = 'test/id/some.Test';
const MOCK_VARIANT_DEF = { def: { key1: 'val1' } };

describe('<AndroidTestTimeline />', () => {
  let mockRootInvocation: RootInvocation;
  let mockTestVariant: TestVariant;

  beforeEach(() => {
    jest.clearAllMocks();

    mockRootInvocation = RootInvocation.fromPartial({
      name: 'rootInvocations/root-123',
      realm: `${MOCK_PROJECT_ID}:some-realm`,
      rootInvocationId: 'root-123',
      primaryBuild: {
        androidBuild: {
          buildId: '888888',
          branch: 'git_main',
          buildTarget: 'target_foo',
        },
      },
      sources: {
        submittedAndroidBuild: {
          buildId: '888888',
          branch: 'git_main',
          dataRealm: 'prod',
        },
        changelists: [],
        isDirty: false,
      },
      createTime: '2025-01-01T12:00:00Z',
    });

    mockTestVariant = TestVariant.fromPartial({
      testId: MOCK_TEST_ID,
      variant: MOCK_VARIANT_DEF,
    });

    // Default mocks
    mockUseListBuilds.mockReturnValue({
      data: { builds: [{ buildId: '999999', creationTimestamp: '2000' }] }, // Default "Latest" build
      isLoading: false,
    });
    mockUseGetTestResultFluxgateSegmentSummaries.mockReturnValue({
      data: { summaries: [] },
      isLoading: false,
    });
  });

  const renderComponent = (inv: RootInvocation = mockRootInvocation) => {
    return render(
      <FakeContextProvider>
        <InvocationProvider
          project={MOCK_PROJECT_ID}
          invocation={inv}
          rawInvocationId="root-123"
          isLegacyInvocation={false}
        >
          <TestVariantProvider
            testVariant={mockTestVariant as OutputTestVerdict}
            displayStatusString="failed"
          >
            <AndroidTestTimeline />
          </TestVariantProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );
  };

  it('should render loading state when fetching builds', () => {
    mockUseListBuilds.mockReturnValue({ isLoading: true });
    renderComponent();
    expect(screen.getByText('Loading timeline...')).toBeInTheDocument();
  });

  it('should render nothing if missing submittedAndroidBuild or build identifiers', () => {
    const inv = RootInvocation.fromPartial({
      ...mockRootInvocation,
      sources: undefined, // Missing sources
      primaryBuild: undefined, // Missing primary build
    });
    const { container } = renderComponent(inv);
    expect(container).toBeEmptyDOMElement();
  });

  it('should render timeline with segments and ellipses', () => {
    // Mock start build search (for before/after 6mo ago)
    mockUseListBuilds.mockImplementation((params) => {
      // Latest Build call (no timestamp params)
      if (
        !params.start_creation_timestamp &&
        !params.end_creation_timestamp &&
        params.page_size === 1
      ) {
        return {
          data: {
            builds: [{ buildId: '999999', creationTimestamp: '2000' }],
          },
          isLoading: false,
        };
      }
      // Before/After logs
      if (params.end_creation_timestamp) {
        return {
          data: {
            builds: [{ buildId: '111111', creationTimestamp: '1000' }],
          },
          isLoading: false,
        };
      }
      return { data: { builds: [] }, isLoading: false };
    });

    // Mock segments: Newer (2), Current (1), Older (2) -> Total 5
    // Current build is 888888
    const summaries = [
      {
        // Newer 2 (Index 0)
        start_result: { build_id: '999999' },
        end_result: { build_id: '990000' },
        health: { fail_rate: { rate: 0.1 } },
      },
      {
        // Newer 1 (Index 1)
        start_result: { build_id: '989999' },
        end_result: { build_id: '900000' },
        health: { fail_rate: { rate: 0.2 } },
      },
      {
        // Current (Index 2)
        start_result: { build_id: '899999' },
        end_result: { build_id: '888888' }, // Contains 888888
        health: { fail_rate: { rate: 0.5 } },
      },
      {
        // Older 1 (Index 3)
        start_result: { build_id: '888887' },
        end_result: { build_id: '800000' },
        health: { fail_rate: { rate: 0.8 } },
      },
      {
        // Older 2 (Index 4)
        start_result: { build_id: '799999' },
        end_result: { build_id: '700000' },
        health: { fail_rate: { rate: 0.9 } },
      },
    ];

    mockUseGetTestResultFluxgateSegmentSummaries.mockReturnValue({
      data: { summaries: [{ summaries }] },
      isLoading: false,
    });

    renderComponent();

    // Verify displayed segments
    expect(screen.getByText('20% failing')).toBeInTheDocument(); // Newer 1
    expect(screen.getByText('50% failing at invocation')).toBeInTheDocument(); // Current
    expect(screen.getByText('80% failed')).toBeInTheDocument(); // Older 1

    // Verify HIDDEN segments
    expect(screen.queryByText('10% failing')).not.toBeInTheDocument(); // Newer 2 (Hidden)
    expect(screen.queryByText('90% failed')).not.toBeInTheDocument(); // Older 2 (Hidden)

    // Verify Ellipses
    const ellipses = screen.getAllByText('...');
    expect(ellipses.length).toBeGreaterThanOrEqual(2); // Start and End ellipses
  });

  it('should use submittedAndroidBuild.buildId as currentBuildId', () => {
    const inv = RootInvocation.fromPartial({
      ...mockRootInvocation,
      sources: {
        submittedAndroidBuild: {
          buildId: '777777', // Different from primaryBuild
          branch: 'git_main',
          dataRealm: 'prod',
        },
        changelists: [],
        isDirty: false,
      },
    });

    mockUseListBuilds.mockReturnValue({
      data: { builds: [{ buildId: '999999', creationTimestamp: '2000' }] },
      isLoading: false,
    });

    const summaries = [
      {
        start_result: { build_id: '777780' },
        end_result: { build_id: '777770' }, // Covers 777777
        health: { fail_rate: { rate: 0.6 } },
      },
    ];

    mockUseGetTestResultFluxgateSegmentSummaries.mockReturnValue({
      data: { summaries: [{ summaries }] },
      isLoading: false,
    });

    const { getByText } = renderComponent(inv);
    expect(getByText('60% now failing')).toBeInTheDocument();

    // Check if Fluxgate was called with correct range.ending_build_id
    expect(mockUseGetTestResultFluxgateSegmentSummaries).toHaveBeenCalledWith(
      expect.objectContaining({
        range: expect.objectContaining({
          ending_build_id: '999999', // Latest build
        }),
      }),
      expect.anything(),
    );
  });

  it('should handle alphanumeric build IDs without crashing', () => {
    const inv = RootInvocation.fromPartial({
      ...mockRootInvocation,
      sources: {
        submittedAndroidBuild: {
          buildId: 'P111226933',
          branch: 'git_main',
          dataRealm: 'prod',
        },
        changelists: [],
        isDirty: false,
      },
    });

    mockUseListBuilds.mockReturnValue({
      data: { builds: [{ buildId: 'P999999', creationTimestamp: '2000' }] },
      isLoading: false,
    });

    const summaries = [
      {
        start_result: { build_id: 'P111226950' },
        end_result: { build_id: 'P111226930' },
        health: { fail_rate: { rate: 0.5 } },
      },
    ];

    mockUseGetTestResultFluxgateSegmentSummaries.mockReturnValue({
      data: { summaries: [{ summaries }] },
      isLoading: false,
    });

    renderComponent(inv);
    expect(screen.getByText('50% now failing')).toBeInTheDocument();
  });
  it('should display synthetic segment when no history is available', () => {
    // Mock empty summaries
    mockUseGetTestResultFluxgateSegmentSummaries.mockReturnValue({
      data: { summaries: [] },
      isLoading: false,
    });

    // Case 1: UNEXPECTED -> 100% failure rate
    mockTestVariant = {
      ...mockTestVariant,
      statusV2: TestVerdict_Status.FAILED,
    } as OutputTestVerdict;

    const { getByText, rerender } = renderComponent();

    // Should show 100% failure rate for "now failing"
    expect(getByText('100% now failing')).toBeInTheDocument();

    // Case 2: FLAKY -> 50% failure rate (approx)
    mockTestVariant = {
      ...mockTestVariant,
      statusV2: TestVerdict_Status.FLAKY,
    } as OutputTestVerdict;

    rerender(
      <FakeContextProvider>
        <InvocationProvider
          project={MOCK_PROJECT_ID}
          invocation={mockRootInvocation}
          rawInvocationId="root-123"
          isLegacyInvocation={false}
        >
          <TestVariantProvider
            testVariant={mockTestVariant as OutputTestVerdict}
            displayStatusString="flaky"
          >
            <AndroidTestTimeline />
          </TestVariantProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('50% now failing')).toBeInTheDocument();

    // Case 3: EXPECTED -> 0% failure rate (approx)
    mockTestVariant = {
      ...mockTestVariant,
      statusV2: TestVerdict_Status.PASSED,
    } as OutputTestVerdict;

    rerender(
      <FakeContextProvider>
        <InvocationProvider
          project={MOCK_PROJECT_ID}
          invocation={mockRootInvocation}
          rawInvocationId="root-123"
          isLegacyInvocation={false}
        >
          <TestVariantProvider
            testVariant={mockTestVariant as OutputTestVerdict}
            displayStatusString="passed"
          >
            <AndroidTestTimeline />
          </TestVariantProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );
    // "0% now failing" might look weird, maybe check if we just show "0%" or similar
    // Based on previous code: rate=0, "0% now failing"
    expect(screen.getByText('0% now failing')).toBeInTheDocument();
  });

  it('should render blamelist link in changepoints', () => {
    // Mock segments: Current (Index 0, Newest) -> Older (Index 1)
    // We expect a changepoint between them.
    // newer.end_result.build_id = to_id = 900
    // older.start_result.build_id = from_id = 800
    // Url: .../from_id/800/to_id/900/

    const summaries = [
      {
        // Current (Index 0)
        start_result: { build_id: '1000' },
        end_result: { build_id: '900' },
        health: { fail_rate: { rate: 0.1 } },
      },
      {
        // Older (Index 1)
        start_result: { build_id: '800' },
        end_result: { build_id: '700' },
        health: { fail_rate: { rate: 0.8 } },
      },
    ];

    mockUseGetTestResultFluxgateSegmentSummaries.mockReturnValue({
      data: { summaries: [{ summaries }] },
      isLoading: false,
    });

    // Make sure findInvocationSegmentIndex returns 0 (Current)
    // buildId 950 is inside [900, 1000]
    const inv = RootInvocation.fromPartial({
      ...mockRootInvocation,
      sources: {
        submittedAndroidBuild: {
          buildId: '950',
          branch: 'git_main',
          dataRealm: 'prod',
        },
      },
    });

    mockUseListBuilds.mockReturnValue({
      data: { builds: [{ buildId: '2000' }] }, // Latest
      isLoading: false,
    });

    renderComponent(inv);

    // Look for the link
    // The changepoint is between index 0 (Current) and index 1 (Older).
    // olderSegment is index 1. from_id = 800.
    // pointingToSegment is index 0. to_id = 900.
    const expectedUrl =
      'https://android-build.corp.google.com/range_search/cls/from_id/800/to_id/900/';

    // We can find the link by role 'link' or title if added
    const link = screen.getByTitle('View Blamelist');
    expect(link).toHaveAttribute('href', expectedUrl);
  });

  it('should render synthetic segment when branch/target are missing but buildId exists', () => {
    // Modify invocation to have NO branch/target, NO submittedAndroidBuild
    // But keep primaryBuild.androidBuild.buildId
    const inv = RootInvocation.fromPartial({
      ...mockRootInvocation,
      primaryBuild: {
        androidBuild: {
          buildId: 'fallback-888',
          // branch/target undefined
        },
      },
      sources: {
        // submittedAndroidBuild undefined
      },
    });

    mockUseGetTestResultFluxgateSegmentSummaries.mockReturnValue({
      data: { summaries: [] },
      isLoading: false,
    });

    // Mock history fetch to return empty or be disabled (it should be disabled)
    mockUseListBuilds.mockReturnValue({
      data: undefined,
      isLoading: false,
    });

    // We need to pass displayStatusString to TestVariantProvider?
    // It's already in renderComponent helper.
    // Also we need to make sure testVariant status is set.
    mockTestVariant = {
      ...mockTestVariant,
      statusV2: TestVerdict_Status.FAILED,
    } as OutputTestVerdict;

    const { getByText } = renderComponent(inv);

    // Should show synthetic segment with 100% failure rate (FAILED)
    expect(getByText('100% now failing')).toBeInTheDocument();

    expect(mockUseGetTestResultFluxgateSegmentSummaries).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ enabled: false }),
    );
  });

  it('should use hashed AnTS ID when properties are present', () => {
    // 1. Setup RootInvocation with system 'atp' (maps to known scheduler)
    const inv = RootInvocation.fromPartial({
      ...mockRootInvocation,
      definition: {
        name: 'test-definition',
        system: 'atp',
        properties: { def: { key: 'val' } },
      },
    });

    // 2. Setup TestVariant with AnTS properties
    const antsProps = {
      '@type':
        'type.googleapis.com/wireless.android.busytown.proto.TestResultProperties',
      antsTestId: {
        module: 'some_module',
        testClass: 'some_class',
        method: 'some_method',
        moduleParameters: [{ name: 'param1', value: 'value1' }],
      },
    };

    const variantWithProps = TestVariant.fromPartial({
      ...mockTestVariant,
      results: [
        {
          result: {
            properties: antsProps,
          },
        },
      ],
    });

    mockUseGetTestResultFluxgateSegmentSummaries.mockReturnValue({
      data: { summaries: [] },
      isLoading: false,
    });

    // Render with the special invocation and variant
    render(
      <FakeContextProvider>
        <InvocationProvider
          project={MOCK_PROJECT_ID}
          invocation={inv}
          rawInvocationId="root-123"
          isLegacyInvocation={false}
        >
          <TestVariantProvider
            testVariant={variantWithProps as OutputTestVerdict}
            displayStatusString="failed"
          >
            <AndroidTestTimeline />
          </TestVariantProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );

    // 3. Verify Fluxgate called with a hashed ID (not the raw MOCK_TEST_ID)
    // The hash should be a hex string.
    expect(mockUseGetTestResultFluxgateSegmentSummaries).toHaveBeenCalledWith(
      expect.objectContaining({
        test_identifier_ids: [expect.stringMatching(/^[0-9a-f]+$/)],
      }),
      expect.objectContaining({ enabled: true }),
    );

    // Verify it is NOT using the raw ID
    expect(
      mockUseGetTestResultFluxgateSegmentSummaries,
    ).not.toHaveBeenCalledWith(
      expect.objectContaining({
        test_identifier_ids: [MOCK_TEST_ID],
      }),
      expect.anything(),
    );
  });
});
