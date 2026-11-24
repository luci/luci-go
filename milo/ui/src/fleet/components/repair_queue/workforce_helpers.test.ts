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
  formatWorkforceChipDuration,
  formatWorkforceDuration,
} from './workforce_helpers';

describe('workforce_helpers', () => {
  describe('formatWorkforceDuration', () => {
    it('returns dash when duration is undefined', () => {
      expect(formatWorkforceDuration(undefined)).toBe('-');
    });

    it('formats proto seconds into human readable minutes', () => {
      expect(formatWorkforceDuration({ seconds: '1680', nanos: 0 })).toBe(
        '28 min',
      );
    });
  });

  describe('formatWorkforceChipDuration', () => {
    it('returns dash when undefined and 0s when zero', () => {
      expect(formatWorkforceChipDuration(undefined)).toBe('-');
      expect(formatWorkforceChipDuration({ seconds: '0', nanos: 0 })).toBe(
        '0s',
      );
    });

    it('truncates rescaled duration to at most two units', () => {
      expect(formatWorkforceChipDuration({ seconds: '94530', nanos: 0 })).toBe(
        '1 day, 2 hr',
      );
      expect(formatWorkforceChipDuration({ seconds: '3750', nanos: 0 })).toBe(
        '1 hr, 2 min',
      );
    });
  });
});
