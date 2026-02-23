import { render, screen } from '@testing-library/react';
import { DateTime } from 'luxon';

import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { useInvocationAggregationQuery } from '@/test_investigation/components/test_aggregation_viewer/hooks';

import { InvocationHeader } from './invocation_header';

jest.mock('@/test_investigation/components/test_aggregation_viewer/hooks');

// Mock `react-router` Link component
jest.mock('react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

describe('InvocationHeader', () => {
  const mockUseInvocationAggregationQuery =
    useInvocationAggregationQuery as jest.Mock;

  const sampleInvocation: Invocation = {
    name: 'invocations/build-87654321',
    state: 1, // ACTIVE
    createTime: DateTime.fromISO('2025-01-01T12:00:00Z').toString(),
    deadline: '',
    tags: [],
    bigqueryExports: [],
    createdBy: 'user:test@example.com',
    producerResource: 'buildbucket',
    realm: 'test:realm',
    historyOptions: undefined,
    finalizeTime: '',
    includedInvocations: [],
    finalizeStartTime: undefined,
    moduleId: undefined,
    properties: undefined,
    sourceSpec: undefined,
    isSourceSpecFinal: false,
    TestResultVariantUnion: undefined,
    extendedProperties: {},
    isExportRoot: false,
    baselineId: '',
    instructions: undefined,
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders correctly without aggregation data', () => {
    mockUseInvocationAggregationQuery.mockReturnValue({
      data: undefined,
      isLoading: true,
    });

    render(<InvocationHeader invocation={sampleInvocation} />);

    expect(screen.getByText(/Invocation/)).toBeInTheDocument();
    // Should not show any counts
    expect(screen.queryByText(/Failed/)).not.toBeInTheDocument();
  });

  it('renders verdict counts correctly', () => {
    mockUseInvocationAggregationQuery.mockReturnValue({
      data: {
        aggregations: [
          {
            verdictCounts: {
              failed: 5,
              executionErrored: 2,
              flaky: 3,
              passed: 100,
              skipped: 10,
            },
          },
        ],
      },
      isLoading: false,
    });

    render(<InvocationHeader invocation={sampleInvocation} />);

    // Check specific text content
    expect(screen.getByText(/5\s+Failed/)).toBeInTheDocument();
    expect(screen.getByText(/2\s+Execution Errored/)).toBeInTheDocument();
    expect(screen.getByText(/3\s+Flaky/)).toBeInTheDocument();
    expect(screen.getByText(/100\s+Passed/)).toBeInTheDocument();
    expect(screen.getByText(/10\s+Skipped/)).toBeInTheDocument();
  });

  it('renders partial verdict counts', () => {
    mockUseInvocationAggregationQuery.mockReturnValue({
      data: {
        aggregations: [
          {
            verdictCounts: {
              failed: 1,
              passed: 50,
            },
          },
        ],
      },
      isLoading: false,
    });

    render(<InvocationHeader invocation={sampleInvocation} />);

    expect(screen.getByText(/1\s+Failed/)).toBeInTheDocument();
    expect(screen.getByText(/50\s+Passed/)).toBeInTheDocument();
    expect(screen.queryByText(/Execution Errored/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Flaky/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Skipped/)).not.toBeInTheDocument();
  });
});
