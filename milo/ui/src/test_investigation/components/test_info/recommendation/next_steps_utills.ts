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
import { QueryTestVariantStabilityResponse } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';
import { TestVerdict_StatusOverride } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

export interface NextStepsInfo {
  status: SemanticStatusType;
  title: string;
  subtitle: string;
}

/**
 * Retrieves information to suggest next steps based on test failure info.
 */
export function getNextStepsInfo(
  testStability: QueryTestVariantStabilityResponse | undefined,
  testVariant: TestVariant,
): NextStepsInfo | undefined {
  if (!testStability) {
    return undefined;
  }
  const isExonerated =
    testVariant.statusOverride === TestVerdict_StatusOverride.EXONERATED;
  if (isExonerated) {
    return {
      status: 'success',
      title: 'No further investigation needed for this failure.',
      subtitle: 'Test has been exonerated.',
    };
  }

  const isBroken = testStability?.testVariants[0]?.failureRate?.isMet || false;
  const isFlaky = testStability?.testVariants[0]?.flakeRate?.isMet || false;
  if (isBroken) {
    return {
      status: 'warning',
      title:
        'This test failure appears to be broken due to other code submitted to codebase.',
      subtitle: 'Try contacting someone or try again later.',
    };
  }
  if (isFlaky) {
    return {
      status: 'warning',
      title:
        'This test failure appears to be flaky due to other code submitted to codebase.',
      subtitle: 'Try contacting someone.',
    };
  }
  return {
    status: 'info',
    title: 'No next steps identified from test analysis.',
    subtitle: 'If you think there should be next steps, please file feedback.',
  };
}
