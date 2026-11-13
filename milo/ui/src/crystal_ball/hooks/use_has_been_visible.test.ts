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

import { act, renderHook } from '@testing-library/react';

import { useHasBeenVisible } from '@/crystal_ball/hooks/use_has_been_visible';
import { FakeIntersectionObserver } from '@/crystal_ball/tests';

describe('useHasBeenVisible', () => {
  const originalIntersectionObserver = window.IntersectionObserver;
  let element: HTMLDivElement;

  beforeEach(() => {
    FakeIntersectionObserver.reset();
    window.IntersectionObserver = FakeIntersectionObserver;
    element = document.createElement('div');
  });

  afterEach(() => {
    window.IntersectionObserver = originalIntersectionObserver;
  });

  it('returns false until the element intersects the viewport', () => {
    const ref = { current: element };
    const { result } = renderHook(() =>
      useHasBeenVisible(ref, { rootMargin: '100% 0px' }),
    );

    expect(result.current).toBe(false);
    expect(FakeIntersectionObserver.instances).toHaveLength(1);
    expect(FakeIntersectionObserver.instances[0].options?.rootMargin).toBe(
      '100% 0px',
    );

    act(() => FakeIntersectionObserver.instances[0].trigger(false));
    expect(result.current).toBe(false);

    act(() => FakeIntersectionObserver.instances[0].trigger(true));
    expect(result.current).toBe(true);
  });

  it('stays true and stops observing once the element has been visible', () => {
    const ref = { current: element };
    const { result } = renderHook(() => useHasBeenVisible(ref));
    const observer = FakeIntersectionObserver.instances[0];

    act(() => observer.trigger(true));

    expect(result.current).toBe(true);
    expect(observer.disconnected).toBe(true);
  });

  it('returns true immediately when disabled', () => {
    const ref = { current: element };
    const { result } = renderHook(() =>
      useHasBeenVisible(ref, { enabled: false }),
    );

    expect(result.current).toBe(true);
    expect(FakeIntersectionObserver.instances).toHaveLength(0);
  });

  it('disconnects the observer on unmount', () => {
    const ref = { current: element };
    const { unmount } = renderHook(() => useHasBeenVisible(ref));
    const observer = FakeIntersectionObserver.instances[0];

    unmount();

    expect(observer.disconnected).toBe(true);
  });
});
