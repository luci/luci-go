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

import { DateTime, Duration } from 'luxon';

import { Link } from '@/common/models/link';
import { parseProtoDuration } from '@/common/tools/time_utils';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { OutputBuild } from '../types';

export function formatBuilderId(builderId: BuilderID) {
  return `${builderId.project}/${builderId.bucket}/${builderId.builder}`;
}

export const FAILURE_STATUSES = Object.freeze([
  Status.FAILURE,
  Status.INFRA_FAILURE,
]);

export const TERMINAL_STATUSES = Object.freeze([
  Status.SUCCESS,
  ...FAILURE_STATUSES,
  Status.CANCELED,
]);

export function isFailureStatus(status: Status) {
  return FAILURE_STATUSES.includes(status);
}

export function isTerminalStatus(status: Status) {
  return TERMINAL_STATUSES.includes(status);
}

export function isCanary(build: Build) {
  return build.input?.experiments?.includes('luci.buildbucket.canary_software');
}

export function getAssociatedGitilesCommit(build: OutputBuild) {
  return build.output?.gitilesCommit || build.input?.gitilesCommit;
}

export function getRecipeLink(build: OutputBuild): Link | null {
  let csHost = 'source.chromium.org';
  if (build.exe?.cipdPackage?.includes('internal')) {
    csHost = 'source.corp.google.com';
  }
  // TODO(crbug.com/1149540): remove this conditional once the long-term
  // solution for recipe links has been implemented.
  if (build.builder.project === 'flutter') {
    csHost = 'cs.opensource.google';
  }
  const recipeName = build.input?.properties?.['recipe'];
  if (!recipeName) {
    return null;
  }

  return {
    label: recipeName as string,
    url: `https://${csHost}/search/?${new URLSearchParams([
      ['q', `file:recipes/${recipeName}.py`],
    ]).toString()}`,
    ariaLabel: `recipe ${recipeName}`,
  };
}

export interface TimingInfo {
  readonly createTime: DateTime;
  readonly startTime: DateTime | null;
  readonly endTime: DateTime | null;

  readonly pendingDuration: Duration;
  readonly schedulingTimeout: Duration | null;
  /**
   * A build exceeded it's scheduling timeout when
   * - the build is canceled, AND
   * - the build did not enter the execution phase, AND
   * - the scheduling timeout is specified, AND
   * - the pending duration is no less than the scheduling timeout.
   */
  readonly exceededSchedulingTimeout: boolean;

  readonly executionDuration: Duration | null;
  readonly executionTimeout: Duration | null;
  /**
   * A build exceeded it's execution timeout when
   * - the build is canceled, AND
   * - the build had entered the execution phase, AND
   * - the execution timeout is specified, AND
   * - the execution duration is no less than the execution timeout.
   */
  readonly exceededExecutionTimeout: boolean;
}

export function getTimingInfo(build: OutputBuild, now?: DateTime): TimingInfo {
  now = now || DateTime.now();

  const createTime = DateTime.fromISO(build.createTime);
  const startTime = build.startTime ? DateTime.fromISO(build.startTime) : null;
  const endTime = build.endTime ? DateTime.fromISO(build.endTime) : null;
  const schedulingTimeout = build.schedulingTimeout
    ? parseProtoDuration(build.schedulingTimeout)
    : null;
  const executionTimeout = build.executionTimeout
    ? parseProtoDuration(build.executionTimeout)
    : null;

  const pendingDuration = (startTime || endTime || now).diff(createTime);
  const executionDuration = startTime ? (endTime || now).diff(startTime) : null;

  const exceededSchedulingTimeout =
    startTime === null &&
    endTime !== null &&
    schedulingTimeout !== null &&
    pendingDuration >= schedulingTimeout;

  const exceededExecutionTimeout =
    build.status === Status.CANCELED &&
    executionDuration !== null &&
    executionTimeout !== null &&
    executionDuration >= executionTimeout;

  return {
    createTime,
    startTime,
    endTime,
    pendingDuration,
    schedulingTimeout,
    exceededSchedulingTimeout,
    executionDuration,
    executionTimeout,
    exceededExecutionTimeout,
  };
}
