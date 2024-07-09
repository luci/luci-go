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

/**
 * Round the number up to a number in the sorted round numbers using linear
 * search.
 *
 * @param num
 * @param sortedRoundNumbers must be sorted in ascending order.
 * @return the rounded down number, or `num` if all numbers in
 *  `sortedRoundNumbers` are less than `num`.
 */
export function roundUp(num: number, sortedRoundNumbers: readonly number[]) {
  for (const predefined of sortedRoundNumbers) {
    if (num <= predefined) {
      return predefined;
    }
  }

  return num;
}

/**
 * Round the number down to a number in the sorted round numbers using linear
 * search.
 *
 * @param num
 * @param sortedRoundNumbers must be sorted in ascending order.
 * @return the rounded up number, or `num` if all numbers in
 *  `sortedRoundNumbers` are greater than `num`.
 */
export function roundDown(num: number, sortedRoundNumbers: readonly number[]) {
  let lastNum = num;

  for (const predefined of sortedRoundNumbers) {
    if (num < predefined) {
      return lastNum;
    }
    lastNum = predefined;
  }

  return lastNum;
}

/**
 * A range with inclusive start and exclusive end.
 */
export type Range = readonly [number, number];

/**
 * Split range `[start, end]` at multiples of `interval`.
 * The first value is `start`, followed by multiples of `interval` between
 * `(start, end)` in ascending order,  and the last value is `end`.
 *
 * For example `splitRange(103, 202, 20)` returns
 * `[103, 120, 140, 160, 180, 200, 202]`.
 *
 * @param range `range[1]` should be no less than `range[0]`. If not, `range[1]`
 *              is normalized to equal `range[0]`.
 * @param interval must be a positive number.
 */
export function splitRange(range: Range, interval: number): readonly number[] {
  if (interval <= 0) {
    throw new Error('interval must be a position number');
  }
  const [start, end] = range;
  if (start >= end) {
    return [start];
  }
  const alignedSteps = [start];

  let alignedStep = Math.floor(start / interval) * interval;
  alignedStep += interval;
  while (alignedStep < end) {
    alignedSteps.push(alignedStep);
    alignedStep += interval;
  }
  alignedSteps.push(end);

  return alignedSteps;
}
