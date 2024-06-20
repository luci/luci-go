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

import { Status as BuildStatus } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

export const BUILD_FIELD_MASK = Object.freeze([
  'id',
  'builder',
  'builderInfo',
  'number',
  'canceledBy',
  'createTime',
  'startTime',
  'endTime',
  'cancelTime',
  'status',
  'statusDetails',
  'summaryMarkdown',
  'input',
  'output',
  'steps',
  'infra.buildbucket.agent',
  'infra.swarming',
  'infra.resultdb',
  'infra.backend',
  'tags',
  'exe',
  'schedulingTimeout',
  'executionTimeout',
  'gracePeriod',
  'ancestorIds',
  'retriable',
] as const);

export const PARTIAL_BUILD_FIELD_MASK = Object.freeze([
  'id',
  'builder',
  'number',
  'createTime',
  'startTime',
  'endTime',
  'status',
] as const);

export const BUILD_STATUS_CLASS_MAP = Object.freeze({
  [BuildStatus.SCHEDULED]: 'scheduled',
  [BuildStatus.STARTED]: 'started',
  [BuildStatus.SUCCESS]: 'success',
  [BuildStatus.FAILURE]: 'failure',
  [BuildStatus.INFRA_FAILURE]: 'infra-failure',
  [BuildStatus.CANCELED]: 'canceled',
});

export const BUILD_STATUS_DISPLAY_MAP = Object.freeze({
  [BuildStatus.SCHEDULED]: 'scheduled',
  [BuildStatus.STARTED]: 'running',
  [BuildStatus.SUCCESS]: 'succeeded',
  [BuildStatus.FAILURE]: 'failed',
  [BuildStatus.INFRA_FAILURE]: 'infra failed',
  [BuildStatus.CANCELED]: 'canceled',
});

export const PERM_BUILDS_CANCEL = 'buildbucket.builds.cancel';
export const PERM_BUILDS_ADD = 'buildbucket.builds.add';
export const PERM_BUILDS_GET = 'buildbucket.builds.get';
export const PERM_BUILDS_GET_LIMITED = 'buildbucket.builds.getLimited';

export const TERMINATED_STATUSES = Object.freeze([
  BuildStatus.SUCCESS,
  BuildStatus.CANCELED,
  BuildStatus.FAILURE,
  BuildStatus.INFRA_FAILURE,
]);

export const FAILED_STATUSES = Object.freeze([
  BuildStatus.FAILURE,
  BuildStatus.INFRA_FAILURE,
]);
