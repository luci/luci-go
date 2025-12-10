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

import { Box } from '@mui/material';
import { useMemo } from 'react';

import { CompareArtifactLinesResponse_FailureOnlyRange } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { CollapsedLines } from './collapsed_lines';
import { TextBox } from './text_box';

interface LogComparisonViewProps {
  logContent: string;
  failureOnlyRanges: readonly CompareArtifactLinesResponse_FailureOnlyRange[];
  isFullLoading?: boolean;
}

export function LogComparisonView({
  logContent,
  failureOnlyRanges,
  isFullLoading,
}: LogComparisonViewProps) {
  const tooltipContent = useMemo(() => {
    return (
      <>
        These highlighted lines only do not appear in the logs from recent
        passing results.
        <br />
        <br />
        This comparison is a guide only and may occasionally fail to hide some
        lines.
      </>
    );
  }, []);
  const segments = useMemo(() => {
    const lines = logContent.split('\n');
    const newSegments: React.ReactNode[] = [];
    let lastLineRendered = 0;

    for (let i = 0; i < failureOnlyRanges.length; i++) {
      const range = failureOnlyRanges[i];

      // Stop if we've exceeded the available content
      if (lastLineRendered >= lines.length) {
        break;
      }

      // Add a collapsed segment for the lines between the last visible segment and this one.
      if (range.startLine > lastLineRendered) {
        const end = Math.min(range.startLine, lines.length);
        const collapsedLines = lines.slice(lastLineRendered, end);
        if (collapsedLines.length > 0) {
          newSegments.push(
            <CollapsedLines
              key={`c-${lastLineRendered}`}
              lines={collapsedLines}
              isStart={i === 0}
              isEnd={false}
            />,
          );
        }
      }

      // If the failure range starts beyond our content, we can't show it yet.
      if (range.startLine >= lines.length) {
        break;
      }

      // Add the visible, failure-only segment.
      const end = Math.min(range.endLine, lines.length);
      const visibleLines = lines.slice(range.startLine, end);
      if (visibleLines.length > 0) {
        newSegments.push(
          <TextBox
            key={`v-${range.startLine}`}
            lines={visibleLines}
            emphasized
            tooltip={tooltipContent}
          />,
        );
      }
      lastLineRendered = range.endLine;
    }

    // Add any remaining lines at the end as a final collapsed segment.
    if (lastLineRendered < lines.length) {
      const finalCollapsedLines = lines.slice(lastLineRendered);
      newSegments.push(
        <CollapsedLines
          key={`c-${lastLineRendered}`}
          lines={finalCollapsedLines}
          isStart={false}
          isEnd={true}
        />,
      );
    }

    if (isFullLoading) {
      newSegments.push(
        <Box
          key="loading-more"
          sx={{
            p: 2,
            display: 'flex',
            justifyContent: 'center',
            color: 'text.secondary',
            fontStyle: 'italic',
          }}
        >
          Loading more content...
        </Box>,
      );
    }

    return newSegments;
  }, [logContent, failureOnlyRanges, tooltipContent, isFullLoading]);

  return (
    <Box sx={{ border: 'solid 1px #aaa', borderRadius: '3px' }}>{segments}</Box>
  );
}
