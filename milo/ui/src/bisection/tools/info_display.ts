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

import { AnalysisStatus, RerunStatus } from '@/common/services/luci_bisection';

export function displayRerunStatus(rerunStatus: RerunStatus): string {
  switch (rerunStatus) {
    case 'RERUN_STATUS_PASSED':
      return 'Passed';
    case 'RERUN_STATUS_FAILED':
      return 'Failed';
    case 'RERUN_STATUS_IN_PROGRESS':
      return 'In Progress';
    case 'RERUN_STATUS_INFRA_FAILED':
      return 'Infra failed';
    case 'RERUN_STATUS_CANCELED':
      return 'Canceled';
  }
  return 'Unknown';
}

export function displayStatus(status: AnalysisStatus): string {
  switch (status) {
    case 'CREATED':
      return 'Created';
    case 'RUNNING':
      return 'Running';
    case 'FOUND':
      return 'Culprit found';
    case 'NOTFOUND':
      return 'Suspect not found';
    case 'SUSPECTFOUND':
      return 'Suspect found';
    case 'ERROR':
      return 'Error';
  }
  return 'Unknown';
}
