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

const EMPTY_RECT: DOMRectReadOnly = {
  bottom: 0,
  height: 0,
  left: 0,
  right: 0,
  toJSON: () => ({}),
  top: 0,
  width: 0,
  x: 0,
  y: 0,
};

/**
 * A controllable IntersectionObserver for tests. Install it on `window` and
 * call `trigger` on an instance to simulate visibility changes.
 */
export class FakeIntersectionObserver implements IntersectionObserver {
  /** All instances created since the last `reset`. */
  static instances: FakeIntersectionObserver[] = [];

  /** Clears the recorded instances. */
  static reset() {
    FakeIntersectionObserver.instances = [];
  }

  readonly root = null;
  readonly rootMargin: string;
  readonly thresholds: readonly number[] = [0];
  disconnected = false;
  private readonly targets: Element[] = [];

  constructor(
    private readonly callback: IntersectionObserverCallback,
    readonly options?: IntersectionObserverInit,
  ) {
    this.rootMargin = options?.rootMargin ?? '0px';
    FakeIntersectionObserver.instances.push(this);
  }

  disconnect() {
    this.disconnected = true;
  }

  observe(target: Element) {
    this.targets.push(target);
  }

  takeRecords(): IntersectionObserverEntry[] {
    return [];
  }

  unobserve(target: Element) {
    const index = this.targets.indexOf(target);
    if (index >= 0) {
      this.targets.splice(index, 1);
    }
  }

  /**
   * Notifies the callback that all observed targets changed visibility.
   * @param isIntersecting - Whether the targets now intersect the root.
   */
  trigger(isIntersecting: boolean) {
    const entries: IntersectionObserverEntry[] = this.targets.map((target) => ({
      boundingClientRect: EMPTY_RECT,
      intersectionRatio: isIntersecting ? 1 : 0,
      intersectionRect: EMPTY_RECT,
      isIntersecting,
      rootBounds: null,
      target,
      time: 0,
    }));
    this.callback(entries, this);
  }
}
