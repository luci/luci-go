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
import { destroy, getSnapshot, Instance, types } from 'mobx-state-tree';
import { fromPromise, FULFILLED, PENDING } from 'mobx-utils';

import { aliveFlow } from './milo_mobx_utils';
import { deferred } from './utils';

const TestStore = types
  .model('TestStore', {
    prop: 0,
  })
  .actions((self) => ({
    aliveAction: aliveFlow(self, function* (promises: Promise<number>[]) {
      for (const promise of promises) {
        self.prop = yield promise;
      }
    }),
  }));

describe('aliveFlow', () => {
  let store: Instance<typeof TestStore>;
  beforeEach(() => {
    jest.useFakeTimers();
    store = TestStore.create({});
  });

  afterEach(() => {
    destroy(store);
    jest.useRealTimers();
  });

  it('when the store is not destroyed', async () => {
    const [promise1, resolve1] = deferred<number>();
    const [promise2, resolve2] = deferred<number>();
    const [promise3, resolve3] = deferred<number>();

    const actionPromise = fromPromise(store.aliveAction([promise1, promise2, promise3]));

    expect(store.prop).toStrictEqual(0);

    resolve1(1);
    await jest.runAllTimersAsync();
    expect(store.prop).toStrictEqual(1);

    resolve2(2);
    await jest.runAllTimersAsync();
    expect(store.prop).toStrictEqual(2);

    resolve3(3);
    await jest.runAllTimersAsync();
    expect(store.prop).toStrictEqual(3);

    await jest.advanceTimersByTimeAsync(100);
    expect(actionPromise.state).toStrictEqual(FULFILLED);
  });

  it('when the store is destroyed while running the action', async () => {
    const [promise1, resolve1] = deferred<number>();
    const [promise2, resolve2] = deferred<number>();
    const [promise3, resolve3] = deferred<number>();

    const actionPromise = fromPromise(store.aliveAction([promise1, promise2, promise3]));

    expect(store.prop).toStrictEqual(0);

    resolve1(1);
    await jest.runAllTimersAsync();
    expect(store.prop).toStrictEqual(1);

    destroy(store);

    resolve2(2);
    await jest.runAllTimersAsync();
    // Use getSnapshot to avoid triggering "reading from dead tree" warning.
    expect(getSnapshot(store).prop).toStrictEqual(1);

    resolve3(3);
    await jest.runAllTimersAsync();
    // Use getSnapshot to avoid triggering "reading from dead tree" warning.
    expect(getSnapshot(store).prop).toStrictEqual(1);

    await jest.advanceTimersByTimeAsync(100);
    expect(actionPromise.state).toStrictEqual(PENDING);
  });
});
