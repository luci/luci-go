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
 * class LazyLoadingElement extends MobxLitElement implements ObserverElement {
 *   @observable.ref private prerender = true;
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
 * class LazyLoadingElement extends MobxLitElement implements RenderPlaceHolder {
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
 * @provideNotifier() to provide a custom notifier.
 * ```
 * @customElement('parent-element')
 * @provider
 * class ParentElement extends MobxLitElement {
 *   @observable.ref
 *   @provideNotifier()
 *   notifier = new ProgressiveNotifier();
 *
 *   protected render() {
 *     return html`<lazy-loading-element></lazy-loading-element>`;
 *   }
 * }
 * ```
 */

import { MobxLitElement } from '@adobe/lit-mobx';
import merge from 'lodash-es/merge';
import { action, makeObservable, observable } from 'mobx';

import { consumer, createContextLink } from '@/generic_libs/tools/lit_context';
import { Constructor } from '@/generic_libs/types';

export interface ObserverElement extends MobxLitElement {
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
  private readonly observer: IntersectionObserver;

  constructor(private readonly options?: IntersectionObserverInit) {
    this.observer = new IntersectionObserver(
      (entries) =>
        entries
          .filter((entry) => entry.isIntersecting)
          .forEach((entry) => {
            (entry.target as ObserverElement).notify();
            this.unsubscribe(entry.target as ObserverElement);
          }),
      this.options,
    );
  }

  subscribe(ele: ObserverElement) {
    this.observer.observe(ele);
  }
  unsubscribe(ele: ObserverElement) {
    this.observer.unobserve(ele);
  }
}

export interface ProgressiveNotifierOption extends IntersectionObserverInit {
  /**
   * How frequent the notifier should notify the remaining elements.
   */
  batchInterval: number;

  /**
   * How many elements should be notified in each batch.
   */
  batchSize: number;
}

/**
 * Notifies the elements when they intersect with the root element for the first
 * time. If no elements were notified in the last `batchInterval` ms, picks
 * `batchSize` un-notified elements and notifies them.
 */
export class ProgressiveNotifier implements Notifier {
  private readonly options: ProgressiveNotifierOption;
  private readonly observer: IntersectionObserver;
  private readonly elements = new Set<ObserverElement>();
  private timeout = 0;

  constructor(options?: Partial<ProgressiveNotifierOption>) {
    this.options = merge(options || {}, { batchInterval: 100, batchSize: 10 });
    this.observer = new IntersectionObserver((entries) => {
      entries
        .filter((entry) => entry.isIntersecting)
        .forEach((entry) => {
          (entry.target as ObserverElement).notify();
          this.unsubscribe(entry.target as ObserverElement);
        });
      this.resetNotificationSchedule();
    }, this.options);
  }

  private resetNotificationSchedule() {
    window.clearTimeout(this.timeout);
    this.timeout = 0;
    this.scheduleNotification();
  }

  private scheduleNotification() {
    if (
      this.timeout ||
      this.elements.size === 0 ||
      this.options.batchSize <= 0
    ) {
      return;
    }
    this.timeout = window.setTimeout(() => {
      let count = this.options.batchSize;
      for (const ele of this.elements) {
        if (count === 0) {
          break;
        }
        ele.notify();
        this.unsubscribe(ele);
        count--;
      }
      this.resetNotificationSchedule();
    }, this.options.batchInterval);
  }

  subscribe(ele: ObserverElement) {
    this.scheduleNotification();
    this.elements.add(ele);
    this.observer.observe(ele);
  }

  unsubscribe(ele: ObserverElement) {
    this.elements.delete(ele);
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
export function observer<T extends ObserverElement, C extends Constructor<T>>(
  cls: C,
) {
  // TypeScript doesn't allow type parameter in extends or implements
  // position. Cast to Constructor<MobxLitElement> to stop tsc complaining.
  class EnterViewObserverElement extends (cls as Constructor<MobxLitElement>) {
    [connectedCBCalledSymbol] = false;

    set [notifierSymbol](newVal: Notifier) {
      if (this[privateNotifierSymbol] === newVal) {
        return;
      }
      this[privateNotifierSymbol].unsubscribe(
        this as MobxLitElement as ObserverElement,
      );
      this[privateNotifierSymbol] = newVal;

      // If the notifier is updated before or during this.connectedCallback(),
      // don't call this[privateNotifierSymbol].subscribe because it was, or
      // will be, called in this.connectedCallback().
      // We can't use this.isConnected, because this.isConnected is true during
      // this.connectedCallback();
      if (this[connectedCBCalledSymbol]) {
        this[privateNotifierSymbol].subscribe(
          this as MobxLitElement as ObserverElement,
        );
      }
    }

    get [notifierSymbol]() {
      return this[privateNotifierSymbol];
    }

    private [privateNotifierSymbol] = INTERSECTION_NOTIFIER;

    connectedCallback() {
      this[notifierSymbol].subscribe(this as MobxLitElement as ObserverElement);
      super.connectedCallback();
      this[connectedCBCalledSymbol] = true;
    }

    disconnectedCallback() {
      this[connectedCBCalledSymbol] = false;
      super.disconnectedCallback();
      this[notifierSymbol].unsubscribe(
        this as MobxLitElement as ObserverElement,
      );
    }
  }

  // Babel doesn't support mixing decorators and properties/methods with
  // computed names.
  // Apply the decorators manually instead.
  consumeNotifier()(EnterViewObserverElement.prototype, notifierSymbol);
  return consumer(
    // Recover the type information that lost in the down-casting above.
    EnterViewObserverElement as Constructor<MobxLitElement> as C,
  );
}

export interface RenderPlaceHolder extends MobxLitElement {
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
export function lazyRendering<
  T extends RenderPlaceHolder,
  C extends Constructor<T>,
>(cls: C) {
  // TypeScript doesn't allow type parameter in extends or implements
  // position. Cast to Constructor<MobxLitElement> to stop tsc complaining.
  class LazilyRenderedElement extends (cls as Constructor<RenderPlaceHolder>) {
    [prerenderSymbol] = true;

    constructor() {
      super();
      makeObservable(this);
    }

    @action notify() {
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

  // Babel doesn't support mixing decorators and properties/methods with
  // computed names.
  // Apply the decorators manually instead.
  observable.ref(LazilyRenderedElement.prototype, prerenderSymbol);
  return observer(
    LazilyRenderedElement,
    // Recover the type information that lost in the down-casting above.
  ) as Constructor<MobxLitElement> as C;
}
