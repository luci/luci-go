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
  Subgraph,
  PrincipalKind,
} from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

export function createMockSubgraph(name: string) {
  const subgraph: Subgraph = {
    nodes: [
      {
        principal: {
          kind: PrincipalKind.GROUP,
          name: name,
        },
        includedBy: [1],
      },
      {
        principal: {
          kind: PrincipalKind.GROUP,
          name: 'nestedGroup',
        },
        includedBy: [2],
      },
      {
        principal: {
          kind: PrincipalKind.GROUP,
          name: 'nestedGroup2',
        },
        includedBy: [3],
      },

      {
        principal: {
          kind: PrincipalKind.GROUP,
          name: 'owningGroup',
        },
        includedBy: [],
      },
    ],
  };

  return subgraph;
}

export function mockFetchGetSubgraph(mockSubgraph: Subgraph) {
  fetchMock.post(
    'https://' +
      SETTINGS.authService.host +
      '/prpc/auth.service.Groups/GetSubgraph',
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body: ")]}'\n" + JSON.stringify(Subgraph.toJSON(mockSubgraph)),
    },
    { overwriteRoutes: true },
  );
}

export function mockErrorFetchingGetSubgraph() {
  fetchMock.post(
    'https://' +
      SETTINGS.authService.host +
      '/prpc/auth.service.Groups/GetSubgraph',
    {
      headers: {
        'X-Prpc-Grpc-Code': '2',
      },
    },
    { overwriteRoutes: true },
  );
}
