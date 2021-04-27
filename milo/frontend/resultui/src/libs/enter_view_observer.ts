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
 * that can react to EnterView event.
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
 *
 * @enterViewObserver() can be used to build components that supports lazy
 * rendering. Alternatively, you can use @lazyRendering() directly.
 * ```
 * @customElement('lazy-loading-element')
 * @lazyRendering()
 * class LazyLoadingElement extends LitElement implements RenderPlaceHolder {
 *   renderPlaceHolder() {
 *     return html`Placeholder`;
 *   }
 *
 *   protected render() {
 *     return html`Actual content`;
 *   }
 * }
 * ```
 */

import { LitElement, property } from 'lit-element';

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
    class EnterViewObserverElement extends (cls as Constructor<LitElement>) {
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
    return (EnterViewObserverElement as Constructor<LitElement>) as C;
  };
}

export interface RenderPlaceHolder extends LitElement {
  /**
   * Renders a placeholder. The placeholder should have roughly the same size
   * as the actual content.
   */
  renderPlaceHolder(): unknown;
}

/**
 * Builds a lazyRenderingMixin, which is a mixin function that takes a
 * constructor that implements RenderPlaceHolder and make the component only
 * renders a placeholder until it intersects with the root element. See
 * @fileoverview for examples.
 */
export function lazyRendering<T extends RenderPlaceHolder>(getNotifier = (_ele: T) => DEFAULT_NOTIFIER) {
  return function lazyRenderingMixin<C extends Constructor<T>>(cls: C) {
    const prerenderSymbol = Symbol('prerender');

    // Recover the type information that lost in the down-casting below.
    @enterViewObserver<LazilyRenderedElement>(getNotifier as (ele: RenderPlaceHolder) => EnterViewNotifier)
    // TypeScript doesn't allow type parameter in extends or implements
    // position. Cast to Constructor<LitElement> to stop tsc complaining.
    class LazilyRenderedElement extends (cls as Constructor<RenderPlaceHolder>) {
      @property() [prerenderSymbol] = true;

      onEnterView() {
        this[prerenderSymbol] = false;
        return true;
      }

      protected render() {
        if (this[prerenderSymbol]) {
          return this.renderPlaceHolder();
        }
        return super.render();
      }
    }
    // Recover the type information that lost in the down-casting above.
    return (LazilyRenderedElement as Constructor<LitElement>) as C;
  };
}
