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

import fetchMock from 'fetch-mock-jest';

import { AuthGroup, ListGroupsResponse } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

export function createMockListGroupsResponse(
  groups: readonly AuthGroup[],
) {
  return ListGroupsResponse.fromPartial({
    groups: groups,
  });
}

export function mockFetchGroups(
  mockGroups: readonly AuthGroup[],
) {
  fetchMock.post(
    'https://' +
      SETTINGS.authService.host +
      '/prpc/auth.service.Groups/ListGroups',
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body:
        ")]}'\n" +
        JSON.stringify(
          ListGroupsResponse.toJSON(
            createMockListGroupsResponse(mockGroups),
          ),
        ),
    },
    { overwriteRoutes: true },
  );
}

export function mockErrorFetchingGroups() {
  fetchMock.post(
    'https://' +
      SETTINGS.authService.host +
      '/prpc/auth.service.Groups/ListGroups',
    {
      headers: {
        'X-Prpc-Grpc-Code': '2',
      },
    },
  );
}
