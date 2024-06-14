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

export function createMockGroup(name: string) {
  return AuthGroup.fromPartial({
    name: name,
    description: 'testDescription',
    owners: 'testOwner',
    createdTs: '1972-01-01T10:00:20.021Z',
    createdBy: 'user:test@example.com',
    callerCanModify: true,
    etag: '123',
  });
}
