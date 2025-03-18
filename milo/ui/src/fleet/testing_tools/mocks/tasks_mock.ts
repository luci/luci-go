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

import fetchMock from 'fetch-mock-jest';

import {
  TaskListResponse,
  TaskResultResponse,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

// TODO: b/404534941 Use `DEVICE_TASKS_SWARMING_HOST` instead.
const LIST_BOT_TASKS_ENDPOINT = `https://${SETTINGS.swarming.defaultHost}/prpc/swarming.v2.Bots/ListBotTasks`;

export function createMockTaskListResponse(tasks: TaskResultResponse[]) {
  return TaskListResponse.fromPartial({ items: tasks });
}

export function mockListBotTasks(tasks: TaskResultResponse[]) {
  fetchMock.post(LIST_BOT_TASKS_ENDPOINT, {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body:
      ")]}'\n" +
      JSON.stringify(
        TaskListResponse.toJSON(createMockTaskListResponse(tasks)),
      ),
  });
}

export function mockErrorListingBotTasks(errorMsg: string) {
  fetchMock.post(LIST_BOT_TASKS_ENDPOINT, {
    headers: {
      'X-Prpc-Grpc-Code': '2',
    },
    body: errorMsg,
  });
}
