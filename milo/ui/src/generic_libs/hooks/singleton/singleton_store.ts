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

export interface SingletonHandle<T> {
  /**
   * Get the value of the singleton attached to this handle.
   *
   * If the handle is disconnected from the store (i.e. the singleton had no
   * subscriber), a cached value will still be returned. But the value might
   * be different from the one currently stored in the singleton store.
   */
  readonly getValue: () => T;
  /**
   * Subscribes the subscriber to the singleton.
   *
   * If the handle was disconnected from the store (i.e. the singleton had no
   * subscribers), and the store currently has a newer value for the singleton,
   * the value attached to this handle will be updated to match the value
   * currently stored in the singleton store.
   *
   * If the handle was disconnected from the store (i.e. the singleton had no
   * subscribers), and the store currently does not have a newer value for the
   * singleton, the value attached to this handle will be stored in the
   * singleton store.
   */
  readonly subscribe: (subscriber: unknown) => void;
  /**
   * Unsubscribes the subscriber from the singleton.
   *
   * If the subscriber is the last subscriber to the singleton, the singleton
   * will be removed from the singleton store. And the handle will be
   * disconnected from the singleton store. When the next subscriber requests
   * the a singleton from the same factory, a new instance will be created.
   * Therefore the value stored in the singleton store will differ from the
   * value attached to this handle.
   */
  readonly unsubscribe: (subscriber: unknown) => void;
}

interface SingletonState<T> extends SingletonHandle<T> {
  /**
   * When defined, all the operations on `this` state should be redirected to
   * the `redirectTo` state.
   */
  redirectTo?: SingletonState<T>;
  /**
   * Keeps track of the value attached to the handler.
   *
   * NOOP when `redirectTo` is defined.
   */
  readonly value: T;
  /**
   * Keeps track of the active subscribers.
   *
   * NOOP when `redirectTo` is defined.
   */
  readonly subscribers: Set<unknown>;
}

/**
 * A store that can manage singletons created from arbitrary factories.
 */
export class SingletonStore<T> {
  private readonly items = new Map<string, SingletonState<T>>();

  /**
   * Get a memorize value created from the same `factory`. If there's no such
   * value, create a new value from the `factory` and memorize it.
   *
   * Then subscribe the `subscriber` to the value.
   * The value is memorized until there's no subscriber for the value created
   * from the same factory.
   */
  getOrCreate(
    subscriber: unknown,
    key: string,
    factory: () => T,
  ): SingletonHandle<T> {
    const items = this.items;

    // Get or create the singleton state from the store.
    let state = items.get(key);
    if (!state) {
      // If there's no memorized value for the key, initialize the state and
      // add it to the tree.
      state = {
        value: factory(),
        subscribers: new Set<unknown>([subscriber]),
        // Implements `SingletonHandle<T>`.
        getValue() {
          if (this.redirectTo) {
            return this.redirectTo.getValue();
          }

          return this.value;
        },
        // Implements `SingletonHandle<T>`.
        subscribe(subscriber: unknown) {
          if (this.redirectTo) {
            return this.redirectTo.subscribe(subscriber);
          }

          if (this.subscribers.size === 0) {
            // If the handle is detached from the store (i.e. there's no
            // subscribers), we should check whether there's new value created
            // from the same factory in the store.
            const state = items.get(key);
            if (state !== undefined) {
              // If yes, the handle should point to the new handle in the store.
              this.redirectTo = state;
              this.redirectTo.subscribe(subscriber);
            } else {
              // If no, we should store this handle in the store so it gets used
              // from now on.
              items.set(key, this);
              this.subscribers.add(subscriber);
            }
          } else {
            // If the handle is not detached from the store, simply add the new
            // subscriber to the set.
            this.subscribers.add(subscriber);
          }
        },
        // Implements `SingletonHandle<T>`.
        unsubscribe(subscriber: unknown) {
          if (this.redirectTo) {
            return this.redirectTo.unsubscribe(subscriber);
          }

          if (
            // We don't need to perform clean up if
            // 1. the subscriber never subscribed to the singleton anyway, or
            !this.subscribers.delete(subscriber) ||
            // 2. the singleton has other subscribers.
            this.subscribers.size > 0
          ) {
            return;
          }
          items.delete(key);
        },
      };

      // Store the state/handle in the store.
      items.set(key, state);
    } else {
      // If there's an existing state/handle, just add the new subscriber to
      // the set.
      state.subscribe(subscriber);
    }

    return state;
  }
}
