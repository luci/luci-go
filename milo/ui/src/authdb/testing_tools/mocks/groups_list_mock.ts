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

import {
  AuthGroup,
  ListGroupsResponse,
} from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';
import { mockFetchRaw } from '@/testing_tools/jest_utils';

export function createMockListGroupsResponse(groups: readonly AuthGroup[]) {
  return ListGroupsResponse.fromPartial({
    groups: groups,
  });
}

export function mockFetchGroups(mockGroups: readonly AuthGroup[]) {
  mockFetchRaw(
    (url) => url.includes('auth.service.Groups/ListGroups'),
    ")]}'\n" +
      JSON.stringify(
        ListGroupsResponse.toJSON(createMockListGroupsResponse(mockGroups)),
      ),
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
        'Content-Type': 'application/json',
      },
    },
  );
}

export function mockErrorFetchingGroups() {
  mockFetchRaw((url) => url.includes('auth.service.Groups/ListGroups'), '', {
    headers: {
      'X-Prpc-Grpc-Code': '2',
    },
  });
}
