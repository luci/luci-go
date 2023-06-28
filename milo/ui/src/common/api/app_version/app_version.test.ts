// Copyright 2023 The LUCI Authors.
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

import { parseAppVersion } from './app_version';

describe('parseVersion', () => {
  test('should parse valid version correctly', async () => {
    const ver = parseAppVersion('13381-5572ca8');
    expect(ver).toEqual({ num: 13381, hash: '5572ca8' });
  });

  test('should parse valid version with suffix correctly', async () => {
    const ver = parseAppVersion('13381-5572ca8-user-branch');
    expect(ver).toEqual({ num: 13381, hash: '5572ca8' });
  });

  test('should throw an error when parsing invalid version', async () => {
    const ver = parseAppVersion('5572ca8-13381');
    expect(ver).toBeNull();
  });
});
