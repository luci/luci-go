// Copyright 2026 The LUCI Authors.
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

import {
  Analysis,
  AnalysisRunStatus,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { AnalysisStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';

export const getStages = (analysis: Analysis) => [
  'Analyzing',
  'Suspect Found',
  'Verifying',
  'Culprit Found',
  analysis.runStatus === AnalysisRunStatus.CANCELED ? 'Canceled' : 'Completed',
];

export function getActiveStep(analysis: Analysis): number {
  const runStatus = analysis.runStatus;
  const status = analysis.status;
  // TODO(jiameil): Clean up this cast once the proto is updated in frontend to include hasTakenActions.
  const hasTakenActions =
    (analysis as unknown as { hasTakenActions?: boolean }).hasTakenActions ||
    false;

  switch (status) {
    case AnalysisStatus.FOUND:
      return hasTakenActions ? 4 : 3;

    case AnalysisStatus.SUSPECTFOUND:
      if (runStatus === AnalysisRunStatus.STARTED) {
        const suspect =
          analysis.genAiResult?.suspect || analysis.nthSectionResult?.suspect;
        const verificationStatus = suspect?.verificationDetails?.status;
        if (
          verificationStatus === 'UNDER_VERIFICATION' ||
          verificationStatus === 'VERIFICATION_SCHEDULED'
        ) {
          return 2;
        }
        return 1;
      }
      if (
        runStatus === AnalysisRunStatus.ENDED ||
        runStatus === AnalysisRunStatus.CANCELED
      ) {
        return 2;
      }
      return -1;

    case AnalysisStatus.CREATED:
    case AnalysisStatus.RUNNING:
      if (runStatus === AnalysisRunStatus.STARTED) {
        return 0;
      }
      if (
        runStatus === AnalysisRunStatus.ENDED ||
        runStatus === AnalysisRunStatus.CANCELED
      ) {
        return 0;
      }
      return -1;

    default:
      if (
        runStatus === AnalysisRunStatus.ENDED ||
        runStatus === AnalysisRunStatus.CANCELED
      ) {
        return 0;
      }
      return -1;
  }
}

export function isStepFailed(analysis: Analysis, stepIndex: number): boolean {
  const runStatus = analysis.runStatus;
  const status = analysis.status;

  if (
    runStatus === AnalysisRunStatus.ENDED ||
    runStatus === AnalysisRunStatus.CANCELED
  ) {
    if (status !== AnalysisStatus.FOUND) {
      return stepIndex === getActiveStep(analysis);
    }
  }
  return false;
}
