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

import {
  buildAbBuildClsUrl,
  buildAbClRangeUrl,
  buildAbClsSinceUrl,
  isSequentialBuildId,
} from './android_build_utils';

const BASE = 'https://android-build.corp.google.com/range_search/cls';

describe('android_build_utils', () => {
  describe('buildAbClRangeUrl', () => {
    it('builds a range URL with include flags and target', () => {
      expect(
        buildAbClRangeUrl({
          fromBuildId: '200',
          includeFrom: true,
          includeTo: false,
          target: 'raven-userdebug',
          toBuildId: '100',
        }),
      ).toBe(
        `${BASE}/from_id/200/to_id/100/?includeFrom=1&includeTo=0&selectedFromTarget=raven-userdebug`,
      );
    });

    it('omits the target when not provided', () => {
      expect(
        buildAbClRangeUrl({
          fromBuildId: '200',
          includeFrom: false,
          includeTo: true,
          toBuildId: '100',
        }),
      ).toBe(`${BASE}/from_id/200/to_id/100/?includeFrom=0&includeTo=1`);
    });

    it('encodes build IDs', () => {
      expect(
        buildAbClRangeUrl({
          fromBuildId: 'a/b',
          includeFrom: true,
          includeTo: true,
          toBuildId: 'c d',
        }),
      ).toBe(`${BASE}/from_id/a%2Fb/to_id/c%20d/?includeFrom=1&includeTo=1`);
    });
  });

  describe('buildAbBuildClsUrl', () => {
    it('builds an inclusive single-build URL', () => {
      expect(buildAbBuildClsUrl('P123', 'target')).toBe(
        `${BASE}/from_id/P123/to_id/P123/?includeFrom=1&includeTo=1&selectedFromTarget=target`,
      );
    });
  });

  describe('buildAbClsSinceUrl', () => {
    it('includes the newer build and excludes the previous build', () => {
      expect(buildAbClsSinceUrl('100', '200', 'target')).toBe(
        `${BASE}/from_id/200/to_id/100/?includeFrom=1&includeTo=0&selectedFromTarget=target`,
      );
    });
  });

  describe('isSequentialBuildId', () => {
    it.each([
      ['12345', true],
      ['P12345', false],
      ['L123', false],
      ['', false],
      ['12a', false],
    ])('isSequentialBuildId(%p) = %p', (id, expected) => {
      expect(isSequentialBuildId(id)).toBe(expected);
    });
  });
});
