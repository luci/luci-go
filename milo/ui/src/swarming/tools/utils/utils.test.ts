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

import { getBuildURLPathFromTags } from './utils';

describe('getBuildURLPathFromTags', () => {
  it('should get URL from buildbucket tag', () => {
    const path = getBuildURLPathFromTags([
      'othertag:value',
      'buildbucket_build_id:123456',
      'othertag2:value',
    ]);
    expect(path).toStrictEqual('/ui/b/123456');
  });

  it('should get URL from logdog tag', () => {
    const path = getBuildURLPathFromTags([
      'othertag:value',
      'log_location:logdog://1234',
      'othertag2:value',
    ]);
    expect(path).toStrictEqual('/raw/build/1234');
  });

  it('should favor buildbucket tag over logdog tag', () => {
    const path = getBuildURLPathFromTags([
      'othertag:value',
      'log_location:logdog://1234',
      'othertag2:value',
      'buildbucket_build_id:123456',
      'othertag3:value',
    ]);
    expect(path).toStrictEqual('/ui/b/123456');
  });

  it("should return null when there's no matching tag", () => {
    const path = getBuildURLPathFromTags(['othertag:value', 'othertag2:value']);
    expect(path).toBeNull();
  });
});
