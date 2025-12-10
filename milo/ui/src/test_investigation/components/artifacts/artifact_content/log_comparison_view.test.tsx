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

import { CompareArtifactLinesResponse_FailureOnlyRange } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { LogComparisonView } from './log_comparison_view';

describe('<LogComparisonView />', () => {
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
      <LogComparisonView
        logContent={partialContent}
        failureOnlyRanges={failureOnlyRanges}
        isFullLoading={true}
      />,
    );

    // Should render visible lines
    expect(screen.getByText(/line 1/)).toBeInTheDocument();
    expect(screen.getByText(/line 2/)).toBeInTheDocument();

    // Should render loading placeholder
    expect(screen.getByText('Loading more content...')).toBeInTheDocument();
  });
});
