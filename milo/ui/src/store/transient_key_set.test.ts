// Copyright 2022 The LUCI Authors.
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

import { afterEach, beforeEach, expect, jest } from '@jest/globals';
import { destroy } from 'mobx-state-tree';

import { TransientKeySet, TransientKeySetInstance } from './transient_key_set';

describe('TransientKeySet', () => {
  let keySet: TransientKeySetInstance;

  beforeEach(() => {
    jest.useFakeTimers();
    keySet = TransientKeySet.create();
  });

  afterEach(() => {
    destroy(keySet);
    jest.useRealTimers();
  });

  it('e2e', async () => {
    const timestamp0 = jest.now();
    keySet.add('key0');
    expect(keySet.has('key0')).toBeTruthy();
    expect(keySet.has('key1')).toBeFalsy();
    expect(keySet.has('key2')).toBeFalsy();

    jest.advanceTimersByTime(1000);
    const timestamp1 = jest.now();
    keySet.add('key1');
    keySet.add('key2');
    expect(keySet.has('key0')).toBeTruthy();
    expect(keySet.has('key1')).toBeTruthy();
    expect(keySet.has('key2')).toBeTruthy();

    jest.advanceTimersByTime(1000);
    const timestamp2 = jest.now();
    keySet.add('key1'); // refresh key1

    jest.advanceTimersByTime(1000);
    const timestamp3 = jest.now();

    // timestamp is exclusive.
    keySet.deleteStaleKeys(new Date(timestamp0));
    expect(keySet.has('key0')).toBeTruthy();
    expect(keySet.has('key1')).toBeTruthy();
    expect(keySet.has('key2')).toBeTruthy();

    keySet.deleteStaleKeys(new Date(timestamp1));
    expect(keySet.has('key0')).toBeFalsy();
    expect(keySet.has('key1')).toBeTruthy();
    expect(keySet.has('key2')).toBeTruthy();

    keySet.deleteStaleKeys(new Date(timestamp2));
    expect(keySet.has('key0')).toBeFalsy();
    expect(keySet.has('key1')).toBeTruthy(); // key1 was refreshed.
    expect(keySet.has('key2')).toBeFalsy();

    keySet.deleteStaleKeys(new Date(timestamp3));
    expect(keySet.has('key0')).toBeFalsy();
    expect(keySet.has('key1')).toBeFalsy();
    expect(keySet.has('key2')).toBeFalsy();
  });
});
