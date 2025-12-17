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

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { CompareArtifactLinesResponse_FailureOnlyRange } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { VirtualizedArtifactContent } from './virtualized_artifact_content';

// Mock @tanstack/react-virtual
jest.mock('@tanstack/react-virtual', () => ({
  useWindowVirtualizer: jest.fn(({ count }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }).map((_, i) => ({
        index: i,
        start: i * 20,
        size: 20,
        measureElement: jest.fn(),
        key: i,
      })),
    getTotalSize: () => count * 20,
    measureElement: jest.fn(),
    options: { scrollMargin: 0 },
  })),
}));

jest.mock('./artifact_content_header', () => ({
  ArtifactContentHeader: jest.fn(() => <div>ArtifactContentHeader</div>),
}));

jest.mock('@/test_investigation/context', () => ({
  useInvocation: jest.fn(() => ({
    name: 'invocations/inv-1',
    realm: 'project:realm',
  })),
}));

describe('<VirtualizedArtifactContent />', () => {
  it('handles partial content with out-of-bounds ranges gracefully', () => {
    const partialContent = 'line 1\nline 2\nline 3';
    const failureOnlyRanges = [
      CompareArtifactLinesResponse_FailureOnlyRange.fromPartial({
        startLine: 0,
        endLine: 2,
      }), // Valid range
      CompareArtifactLinesResponse_FailureOnlyRange.fromPartial({
        startLine: 5,
        endLine: 7,
      }), // Out of bounds range
    ];

    render(
      <VirtualizedArtifactContent
        content={partialContent}
        failureOnlyRanges={failureOnlyRanges}
        isFullLoading={true}
        artifact={
          {
            name: 'invocations/inv-1/artifacts/log.txt',
            artifactId: 'log.txt',
            fetchUrl: 'http://example.com',
          } as Artifact
        }
        hasPassingResults={true}
        isLogComparisonPossible={true}
        showLogComparison={false}
        onToggleLogComparison={jest.fn()}
      />,
    );

    // Should render visible lines
    expect(screen.getByText(/line 1/)).toBeInTheDocument();
    expect(screen.getByText(/line 2/)).toBeInTheDocument();

    // Should render loading placeholder
    expect(screen.getByText('Loading more content...')).toBeInTheDocument();
  });
});
