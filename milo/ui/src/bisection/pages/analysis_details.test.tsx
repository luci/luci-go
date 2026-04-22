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

import {
  getActiveStep,
  getStages,
  isStepFailed,
} from './analysis_progress_utils';

describe('getActiveStep', () => {
  const statuses = [
    AnalysisStatus.ANALYSIS_STATUS_UNSPECIFIED,
    AnalysisStatus.CREATED,
    AnalysisStatus.RUNNING,
    AnalysisStatus.FOUND,
    AnalysisStatus.NOTFOUND,
    AnalysisStatus.ERROR,
    AnalysisStatus.SUSPECTFOUND,
    AnalysisStatus.UNSUPPORTED,
    AnalysisStatus.DISABLED,
    AnalysisStatus.INSUFFICENTDATA,
  ];

  const runStatuses = [
    AnalysisRunStatus.ANALYSIS_RUN_STATUS_UNSPECIFIED,
    AnalysisRunStatus.STARTED,
    AnalysisRunStatus.ENDED,
    AnalysisRunStatus.CANCELED,
  ];

  const testCases: Array<{
    status: AnalysisStatus;
    runStatus: AnalysisRunStatus;
    expected: number;
  }> = [];

  statuses.forEach((status) => {
    runStatuses.forEach((runStatus) => {
      let expected = -1;
      if (status === AnalysisStatus.FOUND) {
        expected = 3;
      } else if (status === AnalysisStatus.SUSPECTFOUND) {
        if (runStatus === AnalysisRunStatus.STARTED) {
          expected = 1;
        } else if (
          runStatus === AnalysisRunStatus.ENDED ||
          runStatus === AnalysisRunStatus.CANCELED
        ) {
          expected = 2;
        }
      } else if (
        status === AnalysisStatus.CREATED ||
        status === AnalysisStatus.RUNNING
      ) {
        if (
          runStatus === AnalysisRunStatus.STARTED ||
          runStatus === AnalysisRunStatus.ENDED ||
          runStatus === AnalysisRunStatus.CANCELED
        ) {
          expected = 0;
        }
      } else {
        if (
          runStatus === AnalysisRunStatus.ENDED ||
          runStatus === AnalysisRunStatus.CANCELED
        ) {
          expected = 0;
        }
      }
      testCases.push({ status, runStatus, expected });
    });
  });

  test.each(testCases)(
    'should return $expected for status $status and runStatus $runStatus',
    ({ status, runStatus, expected }) => {
      const analysis = Analysis.fromPartial({
        status,
        runStatus,
      });
      expect(getActiveStep(analysis)).toBe(expected);
    },
  );

  test('should return 4 if status is FOUND and culprit has actions', () => {
    const analysis = Analysis.fromPartial({
      status: AnalysisStatus.FOUND,
      culprits: [
        {
          culpritAction: [
            {
              actionType: 4,
            },
          ],
        },
      ],
    });
    expect(getActiveStep(analysis)).toBe(4);
  });

  test('should return 2 if status is SUSPECTFOUND and verification is UNDER_VERIFICATION', () => {
    const analysis = Analysis.fromPartial({
      status: AnalysisStatus.SUSPECTFOUND,
      runStatus: AnalysisRunStatus.STARTED,
      genAiResult: {
        suspect: {
          verificationDetails: {
            status: 'UNDER_VERIFICATION',
          },
        },
      },
    });
    expect(getActiveStep(analysis)).toBe(2);
  });

  test('should return 2 if status is SUSPECTFOUND and verification is VERIFICATION_SCHEDULED', () => {
    const analysis = Analysis.fromPartial({
      status: AnalysisStatus.SUSPECTFOUND,
      runStatus: AnalysisRunStatus.STARTED,
      genAiResult: {
        suspect: {
          verificationDetails: {
            status: 'VERIFICATION_SCHEDULED',
          },
        },
      },
    });
    expect(getActiveStep(analysis)).toBe(2);
  });
  test('should return 2 if status is SUSPECTFOUND and verification is Under Verification', () => {
    const analysis = Analysis.fromPartial({
      status: AnalysisStatus.SUSPECTFOUND,
      runStatus: AnalysisRunStatus.STARTED,
      genAiResult: {
        suspect: {
          verificationDetails: {
            status: 'Under Verification',
          },
        },
      },
    });
    expect(getActiveStep(analysis)).toBe(2);
  });

  test('should return 2 if status is SUSPECTFOUND and verification is Verification Scheduled', () => {
    const analysis = Analysis.fromPartial({
      status: AnalysisStatus.SUSPECTFOUND,
      runStatus: AnalysisRunStatus.STARTED,
      genAiResult: {
        suspect: {
          verificationDetails: {
            status: 'Verification Scheduled',
          },
        },
      },
    });
    expect(getActiveStep(analysis)).toBe(2);
  });
});

describe('getStages', () => {
  test('should return Completed as last stage if not canceled', () => {
    const analysis = Analysis.fromPartial({
      runStatus: AnalysisRunStatus.STARTED,
    });
    const stages = getStages(analysis);
    expect(stages[stages.length - 1]).toBe('Completed');
  });

  test('should return Canceled as last stage if canceled', () => {
    const analysis = Analysis.fromPartial({
      runStatus: AnalysisRunStatus.CANCELED,
    });
    const stages = getStages(analysis);
    expect(stages[stages.length - 1]).toBe('Canceled');
  });
});

describe('isStepFailed', () => {
  test('should return true if runStatus is ENDED and status is not FOUND and step is active', () => {
    const analysis = Analysis.fromPartial({
      status: AnalysisStatus.SUSPECTFOUND,
      runStatus: AnalysisRunStatus.ENDED,
    });
    expect(isStepFailed(analysis, 2)).toBe(true);
  });

  test('should return false if runStatus is ENDED and status is not FOUND and step is not active', () => {
    const analysis = Analysis.fromPartial({
      status: AnalysisStatus.SUSPECTFOUND,
      runStatus: AnalysisRunStatus.ENDED,
    });
    expect(isStepFailed(analysis, 0)).toBe(false);
  });

  test('should return false if runStatus is STARTED', () => {
    const analysis = Analysis.fromPartial({
      status: AnalysisStatus.SUSPECTFOUND,
      runStatus: AnalysisRunStatus.STARTED,
    });
    expect(isStepFailed(analysis, 1)).toBe(false);
  });

  test('should return false if status is FOUND', () => {
    const analysis = Analysis.fromPartial({
      status: AnalysisStatus.FOUND,
      runStatus: AnalysisRunStatus.ENDED,
    });
    expect(isStepFailed(analysis, 3)).toBe(false);
  });
});
