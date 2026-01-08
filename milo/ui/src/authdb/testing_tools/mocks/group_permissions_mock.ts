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

import { PrincipalPermissions } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/authdb.pb';
import { mockFetchRaw } from '@/testing_tools/jest_utils';

export function createMockPrincipalPermissions(name: string) {
  const principalPermissions: PrincipalPermissions = {
    name: name,
    realmPermissions: [
      {
        name: 'p:r',
        permissions: ['luci.dev.p1', 'luci.dev.p2'],
      },
      {
        name: 'p:r2',
        permissions: ['luci.dev.p3'],
      },
    ],
  };
  return principalPermissions;
}

export function mockFetchGetPrincipalPermissions(
  mockPermissions: PrincipalPermissions,
) {
  mockFetchRaw(
    (url) => url.includes('auth.service.AuthDB/GetPrincipalPermissions'),
    ")]}'\n" + JSON.stringify(PrincipalPermissions.toJSON(mockPermissions)),
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
        'Content-Type': 'application/json',
      },
    },
  );
}

export function mockErrorFetchingGetPermissions() {
  mockFetchRaw(
    (url) => url.includes('auth.service.AuthDB/GetPrincipalPermissions'),
    '',
    {
      headers: {
        'X-Prpc-Grpc-Code': '2',
      },
    },
  );
}
