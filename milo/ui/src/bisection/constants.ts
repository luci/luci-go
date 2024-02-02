// Copyright 2024 The LUCI Authors.
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

import { BuildFailureType } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { AnalysisStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';
import { SuspectConfidenceLevel } from '@/proto/go.chromium.org/luci/bisection/proto/v1/heuristic.pb';

export const COMPLETE_STATUSES = Object.freeze([
  AnalysisStatus.FOUND,
  AnalysisStatus.NOTFOUND,
  AnalysisStatus.ERROR,
  AnalysisStatus.SUSPECTFOUND,
]);

export const ANALYSIS_STATUS_DISPLAY_MAP = Object.freeze({
  [AnalysisStatus.UNSPECIFIED]: 'UNSPECIFIED',
  [AnalysisStatus.CREATED]: 'CREATED',
  [AnalysisStatus.RUNNING]: 'RUNNING',
  [AnalysisStatus.FOUND]: 'FOUND',
  [AnalysisStatus.NOTFOUND]: 'NOTFOUND',
  [AnalysisStatus.ERROR]: 'ERROR',
  [AnalysisStatus.SUSPECTFOUND]: 'SUSPECTFOUND',
  [AnalysisStatus.UNSUPPORTED]: 'UNSUPPORTED',
  [AnalysisStatus.DISABLED]: 'DISABLED',
  [AnalysisStatus.INSUFFICENTDATA]: 'INSUFFICENTDATA',
});

export const BUILD_FAILURE_TYPE_DISPLAY_MAP = Object.freeze({
  [BuildFailureType.UNSPECIFIED]: 'UNSPECIFIED',
  [BuildFailureType.COMPILE]: 'COMPILE',
  [BuildFailureType.TEST]: 'TEST',
  [BuildFailureType.INFRA]: 'INFRA',
  [BuildFailureType.OTHER]: 'OTHER',
});

export const SUSPECT_CONFIDENCE_LEVEL_DISPLAY_MAP = Object.freeze({
  [SuspectConfidenceLevel.UNSPECIFIED]: 'UNSPECIFIED',
  [SuspectConfidenceLevel.LOW]: 'LOW',
  [SuspectConfidenceLevel.MEDIUM]: 'MEDIUM',
  [SuspectConfidenceLevel.HIGH]: 'HIGH',
});
