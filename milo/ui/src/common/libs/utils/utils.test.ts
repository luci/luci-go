// Copyright 2020 The LUCI Authors.
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

import { expect } from '@jest/globals';

import { extractProperty, roundDown, roundUp, URLExt } from './utils';

describe('URLExt', () => {
  let url: URLExt;
  beforeEach(() => {
    url = new URLExt('https://example.com/path?key1=val1&key2=val2');
  });

  it('should set search params correctly', async () => {
    const newUrlStr = url
      .setSearchParam('key2', 'newVal2')
      .setSearchParam('key3', 'newVal3')
      .toString();
    expect(newUrlStr).toStrictEqual(
      'https://example.com/path?key1=val1&key2=newVal2&key3=newVal3'
    );
  });

  it('should remove matched search params correctly', async () => {
    const newUrlStr = url
      .removeMatchedParams({ key1: 'val1', key2: 'val', key3: 'val3' })
      .toString();
    expect(newUrlStr).toStrictEqual('https://example.com/path?key2=val2');
  });

  it('should not remove search params when multiple values are specified', async () => {
    const url = new URLExt('https://example.com/path?key1=val1&key1=val2');
    const newUrlStr = url.removeMatchedParams({ key1: 'val1' }).toString();
    expect(newUrlStr).toStrictEqual(
      'https://example.com/path?key1=val1&key1=val2'
    );
  });
});

describe('roundUp/Down', () => {
  const list = [1, 3, 5, 7];

  describe('roundUp', () => {
    it('should return the next number in the list', () => {
      expect(roundUp(4, list)).toStrictEqual(5);
    });

    it("should return the number itself if it's in the list", () => {
      expect(roundUp(3, list)).toStrictEqual(3);
    });

    it("should return the number itself if it's larger than any number in the list", () => {
      expect(roundUp(9, list)).toStrictEqual(9);
    });
  });

  describe('roundDown', () => {
    it('should return the next number in the list', () => {
      expect(roundDown(4, list)).toStrictEqual(3);
    });

    it("should return the number itself if it's in the list", () => {
      expect(roundDown(3, list)).toStrictEqual(3);
    });

    it("should return the number itself if it's smaller than any number in the list", () => {
      expect(roundDown(-1, list)).toStrictEqual(-1);
    });
  });
});

describe('extractProperty', () => {
  const obj = {
    a: {
      b: 'd',
      c: ['e', 'f'],
      d: { x: 'nested' },
      '\u1200': 'unicode key',
    },
  };
  it('should return non-null value if path is valid', () => {
    expect(extractProperty(obj, 'a.b')).toStrictEqual('d');
    expect(extractProperty(obj, 'a.c')).toStrictEqual(['e', 'f']);
    expect(extractProperty(obj, 'a.d')).toStrictEqual({ x: 'nested' });
    expect(extractProperty(obj, 'a.\u1200')).toStrictEqual('unicode key');
  });
  it('should return null if path is invalid', () => {
    expect(extractProperty(obj, 'x')).toBeNull();
    expect(extractProperty(obj, 'x.x')).toBeNull();
    expect(extractProperty(obj, 'a.b.x')).toBeNull();
    expect(extractProperty(obj, 'a.c.x')).toBeNull();
  });
});
