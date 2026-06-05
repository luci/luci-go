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

import { extractBuildUrlFromTagData, getBuildUrlFromName } from './builds';

describe('utils/builds', () => {
  describe('extractBuildUrlFromTagData', () => {
    it('handles empty input', () => {
      const result = extractBuildUrlFromTagData([]);
      expect(result).toBeUndefined();
    });

    it('handled invalid tags', () => {
      const result = extractBuildUrlFromTagData([
        'buildbucket_bucket:chromeos/labpack_runner',
        'id:1234',
        'builder:repair',
        'ffsdf:Fdsfds',
      ]);
      expect(result).toBeUndefined();
    });

    it('generates URL when valid tags present', () => {
      const result = extractBuildUrlFromTagData(
        [
          'buildbucket_bucket:chromeos/labpack_runner',
          'buildbucket_build_id:1234',
          'builder:repair',
          'ffsdf:Fdsfds',
        ],
        'localhost:8080',
      );
      expect(result).toEqual(
        'https://localhost:8080/p/chromeos/builders/labpack_runner/repair/b1234/infra',
      );
    });
  });

  describe('getBuildUrlFromName', () => {
    it('returns undefined for non-buildbucket tasks', () => {
      expect(getBuildUrlFromName('some-random-task-name')).toBeUndefined();
    });

    it('generates build URL from matching task name', () => {
      const result = getBuildUrlFromName(
        'bb-8680155568887180465-chromeos/labpack_runner/repair-clank',
        'localhost:8080',
      );
      expect(result).toEqual(
        'https://localhost:8080/p/chromeos/builders/labpack_runner/repair-clank/b8680155568887180465/infra',
      );
    });

    it('generates build URL with default host when miloHost is omitted', () => {
      const result = getBuildUrlFromName(
        'bb-8680155568887180465-chromeos/labpack_runner/repair-clank',
      );
      expect(result).toBeDefined();
      expect(result).toContain(
        '/p/chromeos/builders/labpack_runner/repair-clank/b8680155568887180465/infra',
      );
    });
  });
});
