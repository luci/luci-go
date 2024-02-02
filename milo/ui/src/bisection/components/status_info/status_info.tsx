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

import Typography from '@mui/material/Typography';

import {
  AnalysisStatus,
  RerunStatus,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';

interface LabelProps {
  text: string;
  color: string;
}

const RERUN_STATUS_LABELS: Record<RerunStatus, LabelProps> = {
  [RerunStatus.UNSPECIFIED]: {
    text: 'Unknown',
    color: 'var(--default-text-color)',
  },
  [RerunStatus.PASSED]: {
    text: 'Passed',
    color: 'var(--success-color)',
  },
  [RerunStatus.FAILED]: {
    text: 'Failed',
    color: 'var(--failure-color)',
  },
  [RerunStatus.IN_PROGRESS]: {
    text: 'In progress',
    color: 'var(--started-color)',
  },
  [RerunStatus.INFRA_FAILED]: {
    text: 'Infra failed',
    color: 'var(--critical-failure-color)',
  },
  [RerunStatus.CANCELED]: {
    text: 'Canceled',
    color: 'var(--canceled-color)',
  },
  [RerunStatus.TEST_SKIPPED]: {
    text: 'Test skipped',
    color: 'var(--critical-failure-color)',
  },
};

const ANALYSIS_STATUS_LABELS: Record<AnalysisStatus, LabelProps> = {
  [AnalysisStatus.UNSPECIFIED]: {
    text: 'Unknown',
    color: 'var(--default-text-color)',
  },
  [AnalysisStatus.CREATED]: {
    text: 'Created',
    color: 'var(--scheduled-color)',
  },
  [AnalysisStatus.RUNNING]: { text: 'Running', color: 'var(--started-color)' },
  [AnalysisStatus.FOUND]: {
    text: 'Culprit found',
    color: 'var(--success-color)',
  },
  [AnalysisStatus.NOTFOUND]: {
    text: 'Suspect not found',
    color: 'var(--failure-color)',
  },
  [AnalysisStatus.SUSPECTFOUND]: {
    text: 'Suspect found',
    color: 'var(--suspect-found-color)',
  },
  [AnalysisStatus.ERROR]: {
    text: 'Error',
    color: 'var(--critical-failure-color)',
  },
  [AnalysisStatus.UNSUPPORTED]: {
    text: 'Unsupported',
    color: 'var(--canceled-color)',
  },
  [AnalysisStatus.DISABLED]: {
    text: 'Disabled',
    color: 'var(--canceled-color)',
  },
  [AnalysisStatus.INSUFFICENTDATA]: {
    text: 'Insufficient data',
    color: 'var(--canceled-color)',
  },
};

export interface RerunStatusInfoProps {
  readonly status: RerunStatus;
}

export function RerunStatusInfo({ status }: RerunStatusInfoProps) {
  const statusLabel = RERUN_STATUS_LABELS[status];

  return <Typography color={statusLabel.color}>{statusLabel.text}</Typography>;
}

export interface AnalysisStatusInfoProps {
  status: AnalysisStatus;
}

export function AnalysisStatusInfo({ status }: AnalysisStatusInfoProps) {
  const statusLabel = ANALYSIS_STATUS_LABELS[status];

  return <Typography color={statusLabel.color}>{statusLabel.text}</Typography>;
}
