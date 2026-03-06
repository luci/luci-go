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

import { partition } from './array';

describe('partition', () => {
  it('should split an array into two based on a predicate', () => {
    const arr = [1, 2, 3, 4, 5];
    const predicate = (item: number) => item % 2 === 0;

    const result = partition(arr, predicate);

    expect(result).toEqual([
      [2, 4],
      [1, 3, 5],
    ]);
  });

  it('should handle an array where all items pass the predicate', () => {
    const arr = [2, 4, 6];
    const predicate = (item: number) => item % 2 === 0;

    const result = partition(arr, predicate);

    expect(result).toEqual([[2, 4, 6], []]);
  });

  it('should handle an array where all items fail the predicate', () => {
    const arr = [1, 3, 5];
    const predicate = (item: number) => item % 2 === 0;

    const result = partition(arr, predicate);

    expect(result).toEqual([[], [1, 3, 5]]);
  });

  it('should handle an empty array', () => {
    const arr: number[] = [];
    const predicate = (item: number) => item % 2 === 0;

    const result = partition(arr, predicate);

    expect(result).toEqual([[], []]);
  });
});
