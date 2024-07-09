// Copyright 2024 The LUCI Authors.
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

import { splitRange, roundDown, roundUp, intersect } from './num_utils';

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

describe('splitRange', () => {
  it('can align ticks', () => {
    expect(splitRange([103, 202], 20)).toEqual([
      103, 120, 140, 160, 180, 200, 202,
    ]);
  });

  it('can handle negative interval', () => {
    expect(() =>
      splitRange([103, 202], -20),
    ).toThrowErrorMatchingInlineSnapshot(
      '"interval must be a position number"',
    );
  });

  it('can handle end < start', () => {
    expect(splitRange([202, 103], 20)).toEqual([202]);
  });

  it('can handle end === start', () => {
    expect(splitRange([33, 33], 20)).toEqual([33]);
  });

  it('can handle negative start', () => {
    expect(splitRange([-63, 42], 20)).toEqual([
      -63, -60, -40, -20, 0, 20, 40, 42,
    ]);
  });

  it('can handle negative end', () => {
    expect(splitRange([-63, -42], 20)).toEqual([-63, -60, -42]);
  });

  it('can handle aligned start and end', () => {
    expect(splitRange([-80, 60], 20)).toEqual([
      -80, -60, -40, -20, 0, 20, 40, 60,
    ]);
  });
});

describe('intersect', () => {
  it('subset', () => {
    expect(intersect([10, 20], [5, 25])).toEqual([10, 20]);
  });

  it('superset', () => {
    expect(intersect([5, 25], [10, 20])).toEqual([10, 20]);
  });

  it('overlap', () => {
    expect(intersect([10, 20], [15, 25])).toEqual([15, 20]);
  });

  it('no overlap', () => {
    expect(intersect([10, 20], [25, 25])).toEqual(null);
  });

  it('invalid range', () => {
    expect(intersect([20, 10], [5, 25])).toEqual(null);
  });

  it('empty range', () => {
    expect(intersect([10, 10], [5, 25])).toEqual(null);
  });
});
