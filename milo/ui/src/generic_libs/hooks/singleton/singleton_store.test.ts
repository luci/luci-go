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

import { SingletonStore } from './singleton_store';

describe('SingletonStore', () => {
  it('returns the same instance when the keys are the same', () => {
    const store = new SingletonStore();
    const sub1 = {};
    const sub2 = {};

    const handle1 = store.getOrCreate(sub1, 'key', () => ({}));
    const handle2 = store.getOrCreate(sub2, 'key', () => ({}));

    expect(handle1).toBe(handle2);
  });

  it('returns a new instance when the keys are the different', () => {
    const store = new SingletonStore();
    const sub1 = {};
    const sub2 = {};

    const handle1 = store.getOrCreate(sub1, 'key1', () => ({}));
    const handle2 = store.getOrCreate(sub2, 'key2', () => ({}));

    expect(handle1).not.toBe(handle2);
    expect(handle1.getValue()).not.toBe(handle2.getValue());
  });

  it('unsubscribe can clear the cached instance', () => {
    const store = new SingletonStore();
    const sub1 = {};
    const sub2 = {};

    const handle1 = store.getOrCreate(sub1, 'key', () => ({}));
    handle1.unsubscribe(sub1);

    const handle2 = store.getOrCreate(sub2, 'key', () => ({}));
    expect(handle1).not.toBe(handle2);
    expect(handle1.getValue()).not.toBe(handle2.getValue());
  });

  it('unsubscribe does not clear the cached instance when there are other active subscribers', () => {
    const store = new SingletonStore();

    const sub1 = {};
    const sub2 = {};
    const sub3 = {};
    const handle1 = store.getOrCreate(sub1, 'key', () => ({}));
    const handle2 = store.getOrCreate(sub2, 'key', () => ({}));
    handle1.unsubscribe(sub1);

    const handle3 = store.getOrCreate(sub3, 'key', () => ({}));
    expect(handle3.getValue()).toBe(handle1.getValue());
    expect(handle3).toBe(handle1);
    expect(handle3).toBe(handle2);
  });

  it('re-subscribe works correctly', () => {
    const store = new SingletonStore();

    const sub1 = {};
    const sub2 = {};
    const handle1 = store.getOrCreate(sub1, 'key', () => ({}));
    handle1.unsubscribe(sub1);

    const handle2 = store.getOrCreate(sub2, 'key', () => ({}));
    expect(handle1).not.toBe(handle2);
    const newVal = handle2.getValue();
    expect(handle1.getValue()).not.toBe(newVal);

    // Sub1 resubscribing causes it to get the new value created by sub2.
    handle1.subscribe(sub1);
    expect(handle1.getValue()).toBe(newVal);
    expect(handle2.getValue()).toBe(newVal);

    // Unsubscribe then re-subscribe sub2 should not create a new instance
    // because there was an active subscriber (sub1).
    handle2.unsubscribe(sub2);
    handle2.subscribe(sub2);
    expect(handle1.getValue()).toBe(newVal);
    expect(handle2.getValue()).toBe(newVal);
  });
});
