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

import {
  destroy,
  getSnapshot,
  Instance,
  protect,
  types,
  unprotect,
} from 'mobx-state-tree';
import { fromPromise, FULFILLED, PENDING } from 'mobx-utils';

import { deferred } from '@/generic_libs/tools/utils';
import { silence } from '@/testing_tools/console_filter';

import { aliveFlow, keepAliveComputed } from './mobx_utils';

afterAll(() => {
  // Silence warning from mobx. We will purposefully access dead computed values.
  // The logs from mobx-state-tree happens after all the unit tests and hooks
  // are run. So we cannot recover the mocked `console.warn`. But this should
  // not matter as jest tests from different test files are isolated from each
  // other.
  silence('warn', (err) => `${err}`.includes('[mobx-state-tree]'));
});

describe('aliveFlow', () => {
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

  let store: Instance<typeof TestStore>;
  beforeEach(() => {
    jest.useFakeTimers();
    store = TestStore.create({});
  });

  afterEach(() => {
    destroy(store);
    jest.useRealTimers();
  });

  test('when the store is not destroyed', async () => {
    const [promise1, resolve1] = deferred<number>();
    const [promise2, resolve2] = deferred<number>();
    const [promise3, resolve3] = deferred<number>();

    const actionPromise = fromPromise(
      store.aliveAction([promise1, promise2, promise3]),
    );

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

  test('when the store is destroyed while running the action', async () => {
    const [promise1, resolve1] = deferred<number>();
    const [promise2, resolve2] = deferred<number>();
    const [promise3, resolve3] = deferred<number>();

    const actionPromise = fromPromise(
      store.aliveAction([promise1, promise2, promise3]),
    );

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

describe('keepAliveComputed', () => {
  const TestStore = types
    .model('TestStore', {
      prop: 0,
    })
    .volatile(() => ({
      compute: (_v: number): number => {
        // The function should be mocked be jest.
        throw new Error('unreachable');
      },
    }))
    .views((self) => {
      const computedValue = keepAliveComputed(self, () =>
        self.compute(self.prop),
      );
      return {
        get computedValue() {
          return computedValue.get();
        },
      };
    })
    .actions((self) => ({
      setProp(newVal: number) {
        self.prop = newVal;
      },
    }));

  let store: Instance<typeof TestStore>;
  beforeEach(() => {
    jest.useFakeTimers();
    store = TestStore.create({});
  });

  afterEach(() => {
    unprotect(store);
    destroy(store);
    jest.useRealTimers();
  });

  test('e2e', async () => {
    unprotect(store);
    const computeSpy = jest
      .spyOn(store, 'compute')
      .mockImplementation((v) => v);
    protect(store);

    // Access the computed value.
    expect(store.computedValue).toStrictEqual(0);
    expect(computeSpy).toHaveBeenCalledTimes(1);

    // Accessing the computed value again does not cause it to be re-computed.
    expect(store.computedValue).toStrictEqual(0);
    expect(computeSpy).toHaveBeenCalledTimes(1);

    // Updating the dependency does not cause it to be re-computed.
    store.setProp(1);
    await jest.runAllTimersAsync();
    expect(computeSpy).toHaveBeenCalledTimes(1);

    // Accessing the computed value again after the dependency changed causes it
    // to be re-computed.
    expect(store.computedValue).toStrictEqual(1);
    expect(computeSpy).toHaveBeenCalledTimes(2);

    // We are going to destroy the store. Need to restore mock first to avoid
    // trigger mobx "trying to read or write an object that is no longer part
    // of a state tree" error.
    unprotect(store);
    computeSpy.mockRestore();
    protect(store);

    // Accessing the computed value after the store is destroyed throws an
    // error.
    destroy(store);
    expect(() => store.computedValue).toThrowErrorMatchingInlineSnapshot(
      `"the computed value is accessed when the target node is no longer alive"`,
    );
  });
});
