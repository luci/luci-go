// Copyright 2026 The LUCI Authors.
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

import { getCampusMapsUrl } from './site_helpers';

describe('site_helpers', () => {
  it('normalizes Campus Maps URLs to 3-segment building POI selection links', () => {
    expect(getCampusMapsUrl('US-PAO-EM25')).toBe(
      'https://goto.google.com/where/?id=US-PAO-EM25',
    );
    expect(getCampusMapsUrl('US-PAO-EM25-1')).toBe(
      'https://goto.google.com/where/?id=US-PAO-EM25',
    );
    expect(getCampusMapsUrl('IN-BLR-BCPC-9-9009')).toBe(
      'https://goto.google.com/where/?id=IN-BLR-BCPC',
    );
    expect(getCampusMapsUrl('EM25')).toBe(
      'https://goto.google.com/where/?id=EM25',
    );
    expect(getCampusMapsUrl('   ')).toBeNull();
  });
});
