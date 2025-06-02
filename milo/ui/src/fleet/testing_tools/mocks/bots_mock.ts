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

import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import {
  BotInfo,
  BotInfoListResponse,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

const LIST_BOTS_ENDPOINT = `https://${DEVICE_TASKS_SWARMING_HOST}/prpc/swarming.v2.Bots/ListBots`;

export function createMockListBotsResponse(bots: BotInfo[]) {
  return BotInfoListResponse.fromPartial({ items: bots });
}

export function mockListBots(bots: BotInfo[]) {
  fetchMock.post(LIST_BOTS_ENDPOINT, {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body:
      ")]}'\n" +
      JSON.stringify(
        BotInfoListResponse.toJSON(createMockListBotsResponse(bots)),
      ),
  });
}

export function mockErrorListingBots(errorMsg: string) {
  fetchMock.post(LIST_BOTS_ENDPOINT, {
    headers: {
      'X-Prpc-Grpc-Code': '2',
    },
    body: errorMsg,
  });
}
