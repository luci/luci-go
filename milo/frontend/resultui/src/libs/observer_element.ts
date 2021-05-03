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
 * @observer
 * class LazyLoadingElement extends LitElement implements ObserverElement {
 *   @property() private prerender = true;
 *
 *   notify() {
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
 * @observer can be used to build components that supports lazy rendering.
 * Alternatively, you can use @lazyRendering directly.
 * ```
 * @customElement('lazy-loading-element')
 * @lazyRendering
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
 *
 * By default, enterViewObserver uses the INTERSECTION_NOTIFIER. You can use
 * @provideNotifier to provide a custom notifier.
 * ```
 * @customElement('parent-element')
 * @provider
 * class ParentElement extends LitElement {
 *   @property()
 *   @provideNotifier
 *   notifier = new IntersectionNotifier();
 *
 *   protected render() {
 *     return html`<lazy-loading-element></lazy-loading-element>`;
 *   }
 * }
 * ```
 */

import { LitElement, property } from 'lit-element';

import { consumer, createContextLink } from './context';

export interface ObserverElement extends LitElement {
  notify(): void;
}

export interface Notifier {
  subscribe(ele: ObserverElement): void;
  unsubscribe(ele: ObserverElement): void;
}

export const [provideNotifier, consumeNotifier] = createContextLink<Notifier>();

/**
 * Notifies the elements when they intersect with the root element for the first
 * time.
 */
export class IntersectionNotifier implements Notifier {
  constructor(private readonly options?: IntersectionObserverInit) {}

  private readonly observer = new IntersectionObserver(
    (entries) =>
      entries
        .filter((entry) => entry.isIntersecting)
        .forEach((entry) => {
          (entry.target as ObserverElement).notify();
          this.unsubscribe(entry.target as ObserverElement);
        }),
    this.options
  );

  subscribe(ele: ObserverElement) {
    this.observer.observe(ele);
  }
  unsubscribe(ele: ObserverElement) {
    this.observer.unobserve(ele);
  }
}

export const INTERSECTION_NOTIFIER: Notifier = new IntersectionNotifier();

const notifierSymbol = Symbol('notifier');
const privateNotifierSymbol = Symbol('privateNotifier');
const connectedCBCalledSymbol = Symbol('connectedCBCalled');

/**
 * Ensures the component get notified when it intersects with the root element.
 * See @fileoverview for examples.
 */
export function observer<T extends ObserverElement, C extends Constructor<T>>(cls: C) {
  // TypeScript doesn't allow type parameter in extends or implements
  // position. Cast to Constructor<LitElement> to stop tsc complaining.
  @consumer
  class EnterViewObserverElement extends (cls as Constructor<LitElement>) {
    [connectedCBCalledSymbol] = false;

    @consumeNotifier
    set [notifierSymbol](newVal: Notifier) {
      if (this[privateNotifierSymbol] === newVal) {
        return;
      }
      this[privateNotifierSymbol].unsubscribe((this as LitElement) as ObserverElement);
      this[privateNotifierSymbol] = newVal;

      // If the notifier is updated before or during this.connectedCallback(),
      // don't call this[privateNotifierSymbol].subscribe because it was, or
      // will be, called in this.connectedCallback().
      // We can't use this.isConnected, because this.isConnected is true during
      // this.connectedCallback();
      if (this[connectedCBCalledSymbol]) {
        this[privateNotifierSymbol].subscribe((this as LitElement) as ObserverElement);
      }
    }

    get [notifierSymbol]() {
      return this[privateNotifierSymbol];
    }

    private [privateNotifierSymbol] = INTERSECTION_NOTIFIER;

    connectedCallback() {
      this[notifierSymbol].subscribe((this as LitElement) as ObserverElement);
      super.connectedCallback();
      this[connectedCBCalledSymbol] = true;
    }

    disconnectedCallback() {
      this[connectedCBCalledSymbol] = false;
      super.disconnectedCallback();
      this[notifierSymbol].unsubscribe((this as LitElement) as ObserverElement);
    }
  }
  // Recover the type information that lost in the down-casting above.
  return (EnterViewObserverElement as Constructor<LitElement>) as C;
}

export interface RenderPlaceHolder extends LitElement {
  /**
   * Renders a placeholder. The placeholder should have roughly the same size
   * as the actual content.
   */
  renderPlaceHolder(): unknown;
}

const prerenderSymbol = Symbol('prerender');

/**
 * Makes the component only renders a placeholder until it intersects with the
 * root element. See @fileoverview for examples.
 */
export function lazyRendering<T extends RenderPlaceHolder, C extends Constructor<T>>(cls: C) {
  @observer
  // TypeScript doesn't allow type parameter in extends or implements
  // position. Cast to Constructor<LitElement> to stop tsc complaining.
  class LazilyRenderedElement extends (cls as Constructor<RenderPlaceHolder>) {
    @property() [prerenderSymbol] = true;

    notify() {
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
}
