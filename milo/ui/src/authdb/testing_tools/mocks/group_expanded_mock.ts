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

import { AuthGroup } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

export function createMockExpandedGroup(name: string) {
  return AuthGroup.fromPartial({
    name: name,
    description: 'testDescription',
    members: ['member1@email.com', 'member2@email.com'],
    globs: ['*.com'],
    nested: ['subgroup1', 'subgroup2'],
    owners: 'testOwners',
    createdTs: '2014-06-19T03:17:22.823080Z',
    createdBy: 'user:test@example.com',
    etag: 'W/"MjAyNC0wNC0wMVQyMzoyNjozOS45MDI1MzNa"',
  });
}

export function mockFetchGetExpandedGroup(mockGroup: AuthGroup) {
  fetchMock.post(
    'https://' +
      SETTINGS.authService.host +
      '/prpc/auth.service.Groups/GetExpandedGroup',
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body: ")]}'\n" + JSON.stringify(AuthGroup.toJSON(mockGroup)),
    },
    { overwriteRoutes: true },
  );
}

export function mockErrorFetchingGetExpandedGroup() {
  fetchMock.post(
    'https://' +
      SETTINGS.authService.host +
      '/prpc/auth.service.Groups/GetExpandedGroup',
    {
      headers: {
        'X-Prpc-Grpc-Code': '2',
      },
    },
    { overwriteRoutes: true },
  );
}
