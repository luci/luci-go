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

import { CheckResultStatus, StageResultStatus } from './check_utils';

export type NodeColors = { bg: string; border: string; text: string };

export const COLORS = {
  checkPending: {
    bg: 'var(--light-background-color-1)',
    border: 'var(--light-background-color-4)',
    text: 'var(--default-text-color)',
  },
  checkSuccess: {
    bg: 'var(--success-bg-color)',
    border: 'var(--success-color)',
    text: 'var(--default-text-color)',
  },
  checkFailure: {
    bg: 'var(--failure-bg-color)',
    border: 'var(--failure-color)',
    text: 'var(--default-text-color)',
  },
  checkMixed: {
    bg: 'var(--warning-bg-color)',
    border: 'var(--warning-color)',
    text: 'var(--default-text-color)',
  },
  stagePending: {
    bg: 'var(--scheduled-bg-color)',
    border: 'var(--scheduled-color)',
    text: 'var(--default-text-color)',
  },
  stageSuccess: {
    bg: 'var(--success-bg-color)',
    border: 'var(--success-color)',
    text: 'var(--default-text-color)',
  },
  stageFailure: {
    bg: 'var(--failure-bg-color)',
    border: 'var(--failure-color)',
    text: 'var(--default-text-color)',
  },
  stageRunning: {
    bg: 'var(--started-bg-color)',
    border: 'var(--started-color)',
    text: 'var(--default-text-color)',
  },
  stageCanceled: {
    bg: 'var(--canceled-bg-color)',
    border: 'var(--canceled-color)',
    text: 'var(--default-text-color)',
  },
  collapsedGroup: {
    bg: 'var(--light-background-color-1)',
    border: 'var(--light-background-color-4)',
    text: 'var(--default-text-color)',
  },
};

export function getCheckColors(
  status: CheckResultStatus | undefined,
): NodeColors {
  switch (status) {
    case CheckResultStatus.SUCCESS:
      return COLORS.checkSuccess;
    case CheckResultStatus.FAILURE:
      return COLORS.checkFailure;
    case CheckResultStatus.MIXED:
      return COLORS.checkMixed;
    default:
      return COLORS.checkPending;
  }
}

export function getStageColors(
  status: StageResultStatus | undefined,
): NodeColors {
  switch (status) {
    case StageResultStatus.SUCCESS:
      return COLORS.stageSuccess;
    case StageResultStatus.FAILURE:
      return COLORS.stageFailure;
    case StageResultStatus.RUNNING:
      return COLORS.stageRunning;
    case StageResultStatus.CANCELLED:
      return COLORS.stageCanceled;
    case StageResultStatus.PENDING:
    case StageResultStatus.UNKNOWN:
    default:
      return COLORS.stagePending;
  }
}
