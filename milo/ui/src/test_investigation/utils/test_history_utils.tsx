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

import { SemanticStatusType } from '@/common/styles/status_styles';
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

export function getFormattedFailureRateFromSegment(segment: Segment): string {
  if (!segment || !segment.counts) {
    return 'N/A';
  }
  const { unexpectedVerdicts = 0, totalVerdicts = 0 } = segment.counts;
  const ratePercent =
    totalVerdicts > 0 ? (unexpectedVerdicts / totalVerdicts) * 100 : 0;
  return `${ratePercent.toFixed(0)}%`;
}

export function getFailureRateStatusTypeFromSegment(
  segment: Segment,
): SemanticStatusType {
  if (!segment || !segment.counts) {
    return 'unknown';
  }
  const { unexpectedVerdicts = 0, totalVerdicts = 0 } = segment.counts;
  const ratePercent =
    totalVerdicts > 0 ? (unexpectedVerdicts / totalVerdicts) * 100 : 0;
  return determineRateStatusType(ratePercent);
}

function determineRateStatusType(ratePercent: number): SemanticStatusType {
  if (ratePercent <= 5) return 'passed';
  if (ratePercent > 5 && ratePercent < 95) return 'flaky';
  if (ratePercent >= 95) return 'failed';
  return 'unknown';
}
