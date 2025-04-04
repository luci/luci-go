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

import { ListChangeLogsResponse } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/changelogs.pb';

export function createMockGroupHistory(name: string) {
  const changeLogs: ListChangeLogsResponse = {
    changes: [
      {
        appVersion: '0',
        authDbRev: '0',
        changeType: 'GROUP_MEMBERS_ADDED',
        comment: '',
        configRevNew: '',
        configRevOld: '',
        description: '',
        globs: [],
        identity: '',
        ipAllowList: '',
        members: ['addeduser@email.com'],
        nested: [],
        oauthAdditionalClientIds: [],
        oauthClientId: '',
        oauthClientSecret: '',
        oldDescription: '',
        oldOwners: '',
        owners: '',
        permissionsAdded: [],
        permissionsChanged: [],
        permissionsRemoved: [],
        permsRevNew: '',
        permsRevOld: '',
        securityConfigNew: '',
        securityConfigOld: '',
        subnets: [],
        target: `AuthGroup$${name}`,
        tokenServerUrlNew: '',
        tokenServerUrlOld: '',
        when: '2024-08-15T23:45:29.794195Z',
        who: 'user:user1@email.com',
      },
      {
        appVersion: '0',
        authDbRev: '1',
        changeType: 'GROUP_GLOBS_REMOVED',
        comment: '',
        configRevNew: '',
        configRevOld: '',
        description: '',
        globs: ['*@email.com', '*@gmail.com'],
        identity: '',
        ipAllowList: '',
        members: [],
        nested: [],
        oauthAdditionalClientIds: [],
        oauthClientId: '',
        oauthClientSecret: '',
        oldDescription: '',
        oldOwners: '',
        owners: '',
        permissionsAdded: [],
        permissionsChanged: [],
        permissionsRemoved: [],
        permsRevNew: '',
        permsRevOld: '',
        securityConfigNew: '',
        securityConfigOld: '',
        subnets: [],
        target: `AuthGroup$${name}`,
        tokenServerUrlNew: '',
        tokenServerUrlOld: '',
        when: '2023-08-15T23:45:29.794195Z',
        who: 'user:user2@email.com',
      },
      {
        appVersion: '0',
        authDbRev: '2',
        changeType: 'GROUP_DESCRIPTION_CHANGED',
        comment: '',
        configRevNew: '',
        configRevOld: '',
        description: 'new description',
        globs: [],
        identity: '',
        ipAllowList: '',
        members: [],
        nested: [],
        oauthAdditionalClientIds: [],
        oauthClientId: '',
        oauthClientSecret: '',
        oldDescription: 'old description',
        oldOwners: '',
        owners: '',
        permissionsAdded: [],
        permissionsChanged: [],
        permissionsRemoved: [],
        permsRevNew: '',
        permsRevOld: '',
        securityConfigNew: '',
        securityConfigOld: '',
        subnets: [],
        target: `AuthGroup$${name}`,
        tokenServerUrlNew: '',
        tokenServerUrlOld: '',
        when: '2023-07-15T23:45:29.794195Z',
        who: 'user:user3@email.com',
      },
    ],
    nextPageToken: '',
  };
  return changeLogs;
}

export function mockFetchGetHistory(mockHistory: ListChangeLogsResponse) {
  fetchMock.post(
    'https://' +
      SETTINGS.authService.host +
      '/prpc/auth.service.ChangeLogs/ListChangeLogs',
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body:
        ")]}'\n" + JSON.stringify(ListChangeLogsResponse.toJSON(mockHistory)),
    },
    { overwriteRoutes: true },
  );
}
