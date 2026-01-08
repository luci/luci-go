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

import { AuthGroup } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';
import { mockFetchRaw } from '@/testing_tools/jest_utils';

export function createMockUpdatedGroup(name: string) {
  return AuthGroup.fromPartial({
    name: name,
    description: 'testDescrition',
    members: ['member1', 'member2'],
    nested: ['subgroup1', 'subgroup2'],
    owners: 'testOwners',
    createdTs: '2014-06-19T03:17:22.823080Z',
    createdBy: 'user:test@example.com',
    callerCanModify: true,
    etag: 'W/"MjAyNC0wNC0wMVQyMzoyNjozOS45MDI1MzNa"',
  });
}

export function mockResponseUpdateGroup(mockGroup: AuthGroup) {
  mockFetchRaw(
    (url) => url.includes('auth.service.Groups/UpdateGroup'),
    ")]}'\n" + JSON.stringify(AuthGroup.toJSON(mockGroup)),
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
        'Content-Type': 'application/json',
      },
    },
  );
}

export function mockErrorUpdateGroup() {
  mockFetchRaw((url) => url.includes('auth.service.Groups/UpdateGroup'), '', {
    headers: {
      'X-Prpc-Grpc-Code': '2',
    },
  });
}
