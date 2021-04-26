// Copyright 2021 The LUCI Authors.
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
 * @fileoverview This file contains helper functions for constructing elements
 * that can react to EnterView event. This is useful for building elements that
 * supports lazy rendering.
 *
 * Example:
 * ```
 * @customElement('lazy-loading-element')
 * @enterViewObserver()
 * class LazyLoadingElement extends LitElement implements OnEnterView {
 *   @property() private prerender = true;
 *
 *   onEnterView() {
 *     this.prerender = false;
 *   }
 *
 *   protected render() {
 *     if (this.prerender) {
 *        return html`A place holder`;
 *     }
 *     return html`Actual content`;
 *   }
 * }
 * ```
 */

import { LitElement } from 'lit-element';

export interface OnEnterView extends LitElement {
  onEnterView(): void;
}

/**
 * A special case of IntersectionObserver that notifies the observed elements
 * when they first starting to intersect with the root element.
 */
export class EnterViewNotifier {
  constructor(private readonly options?: IntersectionObserverInit) {}

  private readonly observer = new IntersectionObserver(
    (entries) =>
      entries
        .filter((entry) => entry.isIntersecting)
        .forEach((entry) => {
          (entry.target as OnEnterView).onEnterView();
          this.unobserve(entry.target as OnEnterView);
        }),
    this.options
  );

  // Use composition instead of inheritance so we can force observe/unobserve to
  // take OnEnterViewObserver instead of any element.
  observe = (ele: OnEnterView) => this.observer.observe(ele);
  unobserve = (ele: OnEnterView) => this.observer.unobserve(ele);
}

/**
 * The default enter view notifier that sets the rootMargin to 100px;
 */
const DEFAULT_NOTIFIER = new EnterViewNotifier({ rootMargin: '100px' });

/**
 * Builds a observeEnterViewMixin, which is a mixin function that takes
 * a constructor that implements OnEnterView and ensure it get notified when it
 * intersects with the root element. See @fileoverview for examples.
 */
export function enterViewObserver<T extends OnEnterView>(getNotifier = (_ele: T) => DEFAULT_NOTIFIER) {
  return function observeEnterViewMixin<C extends Constructor<T>>(cls: C) {
    let notifierSymbol: EnterViewNotifier;

    // TypeScript doesn't allow type parameter in extends or implements
    // position. Cast to Constructor<LitElement> to stop tsc complaining.
    class LazyRenderedElement extends (cls as Constructor<LitElement>) {
      connectedCallback() {
        notifierSymbol = getNotifier((this as LitElement) as T);
        notifierSymbol.observe((this as LitElement) as OnEnterView);
        super.connectedCallback();
      }

      disconnectedCallback() {
        super.disconnectedCallback();
        notifierSymbol.unobserve((this as LitElement) as OnEnterView);
      }
    }
    // Recover the type information that lost in the down-casting above.
    return (LazyRenderedElement as Constructor<LitElement>) as C;
  };
}
