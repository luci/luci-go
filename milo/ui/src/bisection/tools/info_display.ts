// Copyright 2023 The LUCI Authors.
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
  AnalysisStatus,
  RerunStatus,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';

export function displayRerunStatus(rerunStatus: RerunStatus): string {
  switch (rerunStatus) {
    case RerunStatus.PASSED:
      return 'Passed';
    case RerunStatus.FAILED:
      return 'Failed';
    case RerunStatus.IN_PROGRESS:
      return 'In Progress';
    case RerunStatus.INFRA_FAILED:
      return 'Infra failed';
    case RerunStatus.CANCELED:
      return 'Canceled';
  }
  return 'Unknown';
}

export function displayStatus(status: AnalysisStatus): string {
  switch (status) {
    case AnalysisStatus.CREATED:
      return 'Created';
    case AnalysisStatus.RUNNING:
      return 'Running';
    case AnalysisStatus.FOUND:
      return 'Culprit found';
    case AnalysisStatus.NOTFOUND:
      return 'Suspect not found';
    case AnalysisStatus.SUSPECTFOUND:
      return 'Suspect found';
    case AnalysisStatus.ERROR:
      return 'Error';
  }
  return 'Unknown';
}
