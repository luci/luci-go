// Copyright 2025 The LUCI Authors.
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

import { GridRowParams } from '@mui/x-data-grid';

import {
  TASK_EXCEPTIONAL_STATES,
  TASK_ONGOING_STATES,
} from '@/fleet/constants/tasks';
import {
  TaskResultResponse,
  TaskState,
  taskStateToJSON,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

import { prettySeconds } from './dates';

// Similar to Swarming's implementation in:
// https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/luci/appengine/swarming/ui2/modules/task-page/task-page-helpers.js;l=100;drc=6c1b10b83a339300fc10d5f5e08a56f1c48b3d3e
// TODO: Look into if we can make the UX for this prettier than what Swarming does.
export const prettifySwarmingState = (task: TaskResultResponse): string => {
  if (task.state === TaskState.COMPLETED) {
    if (task.failure) {
      return 'FAILURE';
    }
    return 'SUCCESS';
  }
  return taskStateToJSON(task.state);
};

// Similar to Swarming's implementation in:
// https://chromium.googlesource.com/infra/luci/luci-py/+/refs/heads/main/appengine/swarming/ui2/modules/task-list/task-list-helpers.js#480
export const getRowClassName = (params: GridRowParams): string => {
  const result = params.row.result;
  if (result === 'FAILURE') {
    return 'row--failure';
  }
  if (TASK_ONGOING_STATES.has(result)) {
    return 'row--pending';
  }
  if (result === 'BOT_DIED') {
    return 'row--bot_died';
  }
  if (result === 'CLIENT_ERROR') {
    return 'row--client_error';
  }
  if (TASK_EXCEPTIONAL_STATES.has(result)) {
    return 'row--exception';
  }
  return '';
};

export const getTaskDuration = (task: TaskResultResponse): string => {
  let duration = task.duration;
  // Running tasks have no duration set, so we can figure it out.
  if (!duration && task.state === TaskState.RUNNING && task.startedTs) {
    duration = (Date.now() - Date.parse(task.startedTs)) / 1000;
  }

  return prettySeconds(duration);
};

export const getTaskTagValue = (
  task: TaskResultResponse,
  tagName: string,
): string | undefined => {
  for (const item of task.tags) {
    const [key, value] = item.split(':');
    if (key && key === tagName && value) {
      return value;
    }
  }

  return undefined;
};
