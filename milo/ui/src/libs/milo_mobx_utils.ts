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

import { computed, IComputedValueOptions, observable } from 'mobx';
import { addDisposer, flow, IAnyStateTreeNode, isAlive } from 'mobx-state-tree';
import { IPromiseBasedObservable, PENDING, REJECTED } from 'mobx-utils';

import { NEVER_PROMISE } from './constants';

/**
 * Unwraps the value in a promise based observable.
 *
 * If the observable is pending, return the defaultValue.
 * If the observable is rejected, throw the error.
 */
export function unwrapObservable<T>(observable: IPromiseBasedObservable<T>, defaultValue: T) {
  switch (observable.state) {
    case PENDING:
      return defaultValue;
    case REJECTED:
      throw observable.value;
    default:
      return observable.value;
  }
}

/**
 * A wrapper around mobx `computed(() => {...}, {..., keepAlive: true})` that
 * ensures the computed value can be properly GCed when `target` is destroyed.
 */
export function keepAliveComputed<T>(
  target: IAnyStateTreeNode,
  func: () => T,
  opts: IComputedValueOptions<T | null> = {}
) {
  const isAlive = observable.box(true);
  const ret = computed(
    () => {
      // Ensure the computed value doesn't observe anything else other than
      // `isAlive` when `target` is no longer alive.
      if (!isAlive.get()) {
        return null;
      }
      return func();
    },
    {
      ...opts,
      keepAlive: true,
    }
  );

  addDisposer(target, () => {
    isAlive.set(false);

    // Re-evaluate the computed value so it no longer have any external
    // dependencies.
    ret.get();
  });
  return ret;
}

/**
 * The same as `flow` from `mobx-state-tree` except that
 * 1. it takes a target node, and
 * 2. it will only call `.next(...)` on the generator when the target is alive.
 */
// Use the same signature as `flow` from `mobx-state-tree`, which uses `any`.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function aliveFlow<R, Args extends any[]>(
  target: IAnyStateTreeNode,
  generator: Parameters<typeof flow<R, Args>>[0]
): ReturnType<typeof flow<R, Args>> {
  return flow(function* (...args) {
    const gen = generator(...args);
    let next = gen.next();
    while (!next.done) {
      const resolved = yield next.value;
      if (!isAlive(target)) {
        yield NEVER_PROMISE;
      }
      next = gen.next(resolved);
    }
    return next.value;
  });
}
