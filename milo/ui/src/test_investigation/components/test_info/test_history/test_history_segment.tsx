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

import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { TestHistorySegmentSummary } from './test_history_segment_summary';
import { TestHistorySourceVerdicts } from './test_history_source_verdicts';
interface TestHistorySegmentProps {
  segment: Segment;
  isStartSegment: boolean;
  isEndSegment: boolean;
  setExpandedTestHistory: (expand: boolean) => void;
  testHistoryHasExpandedSegment: boolean;
}

export function TestHistorySegment({
  segment,
  isStartSegment,
  isEndSegment,
  setExpandedTestHistory,
  testHistoryHasExpandedSegment,
}: TestHistorySegmentProps) {
  return (
    <Box>
      <TestHistorySourceVerdicts
        segment={segment}
        setExpandedTestHistory={setExpandedTestHistory}
        testHistoryHasExpandedSegment={testHistoryHasExpandedSegment}
      ></TestHistorySourceVerdicts>{' '}
      <TestHistorySegmentSummary
        segment={segment}
        isStartSegment={isStartSegment}
        isEndSegment={isEndSegment}
      ></TestHistorySegmentSummary>
    </Box>
  );
}
