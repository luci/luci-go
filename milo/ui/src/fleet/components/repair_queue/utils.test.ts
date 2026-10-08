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

import { formatPoints, getBugUrl } from './utils';

describe('repair queue utils', () => {
  describe('formatPoints helper', () => {
    it('formats positive values with + and pts', () => {
      expect(formatPoints('350')).toBe('+350 pts');
      expect(formatPoints('+350')).toBe('+350 pts');
      expect(formatPoints('1')).toBe('+1 pts');
      expect(formatPoints(300)).toBe('+300 pts');
    });

    it('formats negative values with - and pts', () => {
      expect(formatPoints('-50')).toBe('-50 pts');
      expect(formatPoints(-50)).toBe('-50 pts');
      expect(formatPoints('-9223372036854775808')).toBe(
        '-9223372036854775808 pts',
      );
    });

    it('formats zero values as "0 pts"', () => {
      expect(formatPoints('0')).toBe('0 pts');
      expect(formatPoints('-0')).toBe('0 pts');
      expect(formatPoints(0)).toBe('0 pts');
    });

    it('handles empty or non-numeric strings safely', () => {
      expect(formatPoints('')).toBe('0 pts');
      expect(formatPoints(undefined)).toBe('0 pts');
    });
  });

  describe('getBugUrl', () => {
    it('builds a Buganizer issue URL', () => {
      expect(getBugUrl('12345678')).toBe(
        'https://b.corp.google.com/issues/12345678',
      );
    });

    it('encodes the bug ID', () => {
      expect(getBugUrl('1/2')).toBe('https://b.corp.google.com/issues/1%2F2');
    });
  });
});
