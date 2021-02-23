// Copyright 2021 The LUCI Authors.
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

import { assert } from 'chai';

import { getInvIdFromBuildId, getInvIdFromBuildNum } from './resultdb';

describe('resultdb', () => {
  it('should compute invocation ID from build number correctly', async () => {
    const invId = await getInvIdFromBuildNum({project: 'chromium', bucket: 'ci', builder: 'ios-device'}, 179945);
    assert.strictEqual(invId, 'build-135d246ed1a40cc3e77d8b1daacc7198fe344b1ac7b95c08cb12f1cc383867d7-179945');
  });

  describe('should compute invocation ID from build ID correctly', async () => {
    const invId = await getInvIdFromBuildId('123456');
    assert.strictEqual(invId, 'build-123456');
  });
});
