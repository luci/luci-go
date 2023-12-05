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

import { AnalysisStatus, RerunStatus } from '@/common/services/luci_bisection';

interface LabelProps {
  text: string;
  color: string;
}

const RERUN_STATUS_LABELS: Record<RerunStatus, LabelProps> = {
  RERUN_STATUS_UNSPECIFIED: {
    text: 'Unknown',
    color: 'var(--default-text-color)',
  },
  RERUN_STATUS_PASSED: { text: 'Passed', color: 'var(--success-color)' },
  RERUN_STATUS_FAILED: { text: 'Failed', color: 'var(--failure-color)' },
  RERUN_STATUS_IN_PROGRESS: {
    text: 'In progress',
    color: 'var(--started-color)',
  },
  RERUN_STATUS_INFRA_FAILED: {
    text: 'Infra failed',
    color: 'var(--critical-failure-color)',
  },
  RERUN_STATUS_CANCELED: { text: 'Canceled', color: 'var(--canceled-color)' },
  RERUN_STATUS_TEST_SKIPPED: {
    text: 'Test skipped',
    color: 'var(--critical-failure-color)',
  },
};

const ANALYSIS_STATUS_LABELS: Record<AnalysisStatus, LabelProps> = {
  ANALYSIS_STATUS_UNSPECIFIED: {
    text: 'Unknown',
    color: 'var(--default-text-color)',
  },
  CREATED: { text: 'Created', color: 'var(--scheduled-color)' },
  RUNNING: { text: 'Running', color: 'var(--started-color)' },
  FOUND: { text: 'Culprit found', color: 'var(--success-color)' },
  NOTFOUND: { text: 'Suspect not found', color: 'var(--failure-color)' },
  SUSPECTFOUND: { text: 'Suspect found', color: 'var(--suspect-found-color)' },
  ERROR: { text: 'Error', color: 'var(--critical-failure-color)' },
  UNSUPPORTED: { text: 'Unsupported', color: 'var(--canceled-color)' },
  DISABLED: { text: 'Disabled', color: 'var(--canceled-color)' },
  INSUFFICENTDATA: {
    text: 'Insufficient data',
    color: 'var(--canceled-color)',
  },
};

interface RerunStatusProps {
  status: RerunStatus;
}

export const RerunStatusInfo = ({ status }: RerunStatusProps) => {
  const statusLabel = RERUN_STATUS_LABELS[status];

  return <Typography color={statusLabel.color}>{statusLabel.text}</Typography>;
};

interface AnalysisStatusProps {
  status: AnalysisStatus;
}

export const AnalysisStatusInfo = ({ status }: AnalysisStatusProps) => {
  const statusLabel = ANALYSIS_STATUS_LABELS[status];

  return <Typography color={statusLabel.color}>{statusLabel.text}</Typography>;
};
