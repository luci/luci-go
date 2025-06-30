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

import { getPropertyGroup, parseDeepLinkQuery } from './redirect_utils';

describe('getPropertyGroup', () => {
  it('should return the correct group for testId keys', () => {
    expect(getPropertyGroup('ID:')).toBe('testId');
    expect(getPropertyGroup('ExactID:')).toBe('testId');
  });

  it('should return the correct group for vhash keys', () => {
    expect(getPropertyGroup('VHash:')).toBe('vhash');
  });

  it('should return the correct group for variantDef keys', () => {
    expect(getPropertyGroup('V:')).toBe('variantDef');
  });

  it('should return "unknown" for an unrecognized key', () => {
    expect(getPropertyGroup('UnknownKey:')).toBe('unknown');
    expect(getPropertyGroup('')).toBe('unknown');
  });
});

describe('parseDeepLinkQuery', () => {
  it('should return nulls for null or empty input', () => {
    const expected = { testId: null, variantHash: null, variantDef: null };
    expect(parseDeepLinkQuery(null)).toEqual(expected);
    expect(parseDeepLinkQuery('')).toEqual(expected);
  });

  it('should treat V: keys as separate, distinct properties', () => {
    const query = 'V:key1=val1 V:key2=val2';
    const result = parseDeepLinkQuery(query);
    expect(result.variantDef).toEqual({
      key1: 'val1',
      key2: 'val2',
    });
  });

  it('should handle a complex mix of merged singletons and separate V: keys', () => {
    const query = 'ID:a ID:b V:k1=v1 V:k2=v2 VHash:h1 VHash:h2 ID:c';
    const result = parseDeepLinkQuery(query);
    expect(result).toEqual({
      testId: 'a ID:b', // First 'testId' group wins, 'ID:c' is ignored
      variantHash: 'h1 VHash:h2', // The only 'vhash' group
      variantDef: {
        // Both 'V:' keys are processed separately
        k1: 'v1',
        k2: 'v2',
      },
    });
  });

  it('should handle spaces and encoding correctly', () => {
    const query = 'ID:test%20with%20spaces V:k=a value with spaces';
    const result = parseDeepLinkQuery(query);
    expect(result.testId).toBe('test with spaces');
    expect(result.variantDef).toEqual({ k: 'a value with spaces' });
  });
});
