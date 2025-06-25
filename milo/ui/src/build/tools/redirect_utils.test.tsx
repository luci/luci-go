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
  // Test for null, undefined, and empty string inputs
  it('should return nulls for null or empty input', () => {
    const expected = { testId: null, variantHash: null, variantDef: null };
    expect(parseDeepLinkQuery(null)).toEqual(expected);
    expect(parseDeepLinkQuery('')).toEqual(expected);
  });

  it('should handle a simple query with one of each property', () => {
    const query = 'ID:test-123 VHash:abc-def V:key1=value1';
    const result = parseDeepLinkQuery(query);
    expect(result).toEqual({
      testId: 'test-123',
      variantHash: 'abc-def',
      variantDef: { key1: 'value1' },
    });
  });

  it('should handle spaces within values', () => {
    const query = 'ID:this is my test V:config=use new settings';
    const result = parseDeepLinkQuery(query);
    expect(result).toEqual({
      testId: 'this is my test',
      variantHash: null,
      variantDef: { config: 'use new settings' },
    });
  });

  it('should correctly decode URI-encoded characters', () => {
    const query =
      'ID:test%20with%20spaces V:url=https%3A%2F%2Fexample.com%3Fa%3D1';
    const result = parseDeepLinkQuery(query);
    expect(result).toEqual({
      testId: 'test with spaces',
      variantHash: null,
      variantDef: { url: 'https://example.com?a=1' },
    });
  });

  it('should return null for variantDef if no V: properties are present', () => {
    const query = 'ID:only-an-id VHash:only-a-hash';
    const result = parseDeepLinkQuery(query);
    expect(result.variantDef).toBeNull();
  });

  describe('Key Merging Logic', () => {
    it('should merge subsequent identical keys into the previous value', () => {
      const query = 'ID:value one ID:value two';
      const result = parseDeepLinkQuery(query);
      expect(result.testId).toBe('value one ID:value two');
      expect(result.variantHash).toBeNull();
      expect(result.variantDef).toBeNull();
    });

    it('should merge subsequent keys of the same group (ID: and ExactID:)', () => {
      const query = 'ID:first part ExactID:second part';
      const result = parseDeepLinkQuery(query);
      expect(result.testId).toBe('first part ExactID:second part');
    });

    it('should apply the merging rule to V: keys as well', () => {
      const query = 'V:key1=val1 with spaces V:key2=val2';
      const result = parseDeepLinkQuery(query);
      expect(result.variantDef).toEqual({
        key1: 'val1 with spaces V:key2=val2',
      });
    });

    it('should handle a long chain of mixed and merged properties', () => {
      const query = 'ID:a ID:b V:k1=v1 V:k2=v2 VHash:hash1 VHash:hash2 ID:c';
      const result = parseDeepLinkQuery(query);
      expect(result).toEqual({
        testId: 'a ID:b',
        variantHash: 'hash1 VHash:hash2',
        variantDef: { k1: 'v1 V:k2=v2' },
      });
    });

    it('should correctly restart parsing after a merged block', () => {
      const query = 'ID:a ID:b V:c=d';
      const result = parseDeepLinkQuery(query);
      expect(result.testId).toBe('a ID:b');
      expect(result.variantDef).toEqual({ c: 'd' });
    });
  });

  describe('V: property edge cases', () => {
    it('should ignore a V: property with no equals sign', () => {
      const query = 'V:thisisnotvalid ID:test1';
      const result = parseDeepLinkQuery(query);
      expect(result.testId).toBe('test1');
      expect(result.variantDef).toBeNull();
    });

    it('should ignore a V: property with an equals sign at the beginning', () => {
      const query = 'V:=novalidkey ID:test1';
      const result = parseDeepLinkQuery(query);
      expect(result.testId).toBe('test1');
      expect(result.variantDef).toBeNull();
    });

    it('should handle values containing an equals sign', () => {
      const query = 'V:formula=a=b*2';
      const result = parseDeepLinkQuery(query);
      expect(result.variantDef).toEqual({ formula: 'a=b*2' });
    });
  });

  it('should ignore unrecognized text between properties', () => {
    const query = 'ID:test1 some random text VHash:hash1';
    const result = parseDeepLinkQuery(query);
    expect(result).toEqual({
      testId: 'test1 some random text',
      variantHash: 'hash1',
      variantDef: null,
    });
  });
});
