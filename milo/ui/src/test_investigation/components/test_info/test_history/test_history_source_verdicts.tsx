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
import { useEffect, useState } from 'react';

import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { SourceVerdictsCollapsed } from './source_verdicts_collapsed';
import { SourceVerdictsExpanded } from './source_verdicts_expanded';

interface TestHistorySegmentProps {
  segment: Segment;
  setExpandedTestHistory: (expand: boolean) => void;
  testHistoryHasExpandedSegment: boolean;
  changeNumShownVerdicts: (num: number) => void;
}

export function TestHistorySourceVerdicts({
  segment,
  setExpandedTestHistory,
  testHistoryHasExpandedSegment,
  changeNumShownVerdicts,
}: TestHistorySegmentProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  const expandSegment = () => {
    setIsExpanded(true);
    setExpandedTestHistory(true);
  };

  useEffect(() => {
    // If no segments are expanded in parent container (ie. collapse all has been pressed), collapse this segment.
    if (!testHistoryHasExpandedSegment) {
      setIsExpanded(false);
    }
  }, [testHistoryHasExpandedSegment]);

  const sourceVerdictNumberChanged = (num: number) => {
    changeNumShownVerdicts(num);
  };

  return (
    <>
      {!isExpanded ? (
        <SourceVerdictsCollapsed
          segment={segment}
          expandSegment={expandSegment}
        ></SourceVerdictsCollapsed>
      ) : (
        <SourceVerdictsExpanded
          segment={segment}
          sourceVerdictNumberChanged={sourceVerdictNumberChanged}
        ></SourceVerdictsExpanded>
      )}
    </>
  );
}
