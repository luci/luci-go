// Copyright 2022 The LUCI Authors.
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

import Typography from '@mui/material/Typography';

import { AnalysisStatus, RerunStatus } from '../../services/luci_bisection';

interface RerunStatusProps {
  status: RerunStatus;
}

export const RerunStatusInfo = ({ status }: RerunStatusProps) => {
  switch (status) {
    case 'RERUN_STATUS_PASSED':
      return <Typography color='var(--success-color)'>Passed</Typography>;
    case 'RERUN_STATUS_FAILED':
      return <Typography color='var(--failure-color)'>Failed</Typography>;
    case 'RERUN_STATUS_IN_PROGRESS':
      return <Typography color='var(--started-color)'>In progress</Typography>;
    case 'RERUN_STATUS_INFRA_FAILED':
      return <Typography color='var(--critical-failure-color)'>Infra failed</Typography>;
    case 'RERUN_STATUS_CANCELED':
      return <Typography color='var(--canceled-color)'>Canceled</Typography>;
  }
  return <Typography color='var(--default-text-color)'>Unknown</Typography>;
};

interface AnalysisStatusProps {
  status: AnalysisStatus;
}

export const AnalysisStatusInfo = ({ status }: AnalysisStatusProps) => {
  switch (status) {
    case 'CREATED':
      return <Typography color='var(--scheduled-color)'>Created</Typography>;
    case 'RUNNING':
      return <Typography color='var(--started-color)'>Running</Typography>;
    case 'FOUND':
      return <Typography color='var(--success-color)'>Culprit found</Typography>;
    case 'NOTFOUND':
      return <Typography color='var(--failure-color)'>Suspect not found</Typography>;
    case 'SUSPECTFOUND':
      return <Typography color='var(--suspect-found-color)'>Suspect found</Typography>;
    case 'ERROR':
      return <Typography color='var(--critical-failure-color)'>Error</Typography>;
  }
  return <Typography color='var(--default-text-color)'>Unknown</Typography>;
};
