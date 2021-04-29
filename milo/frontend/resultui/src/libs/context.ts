// Copyright 2020 The LUCI Authors.
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
 * @fileoverview This file contains helper functions for constructing
 * context provider mixin and context consumer mixin.
 * The design is heavily inspired by
 * [React Context](https://reactjs.org/docs/context.html)
 *
 * ## Decorator Example
 * ```
 * @customElement('context-provider')
 * // Specify which context keys to consume, if the corresponding properties
 * // don't exist in the class, a compilation error will be thrown.
 * @provideContext('contextKey1')
 * @provideContext('contextKey2')
 * class ContextProvider extends LitElement {
 *   @property() contextKey1 = 'value';
 *   @property() contextKey2 = 1;
 *
 *   protected render() {
 *     html`
 *       <slot></slot>
 *     `;
 *   }
 * }
 *
 * @customElement('context-consumer')
 * // Specify which context keys to consume, if the corresponding properties
 * // don't exist in the class, a compilation error will be thrown.
 * @consumeContext('contextKey1')
 * class ContextConsumer extends LitElement {
 *   @property() contextKey1 = '';
 *
 *   protected render() {
 *     html`${this.contextKey1}`;
 *   }
 * }
 *
 * @customElement('page')
 * class Page extends LitElement {
 *   // renders 'page-value' on screen.
 *   protected render() {
 *     return html`
 *       <context-provider .contextKey1=${'page-value'}>
 *         <context-consumer>
 *         </context-consumer>
 *       </context-provider>
 *     `;
 *   }
 * }
 * ```
 *
 * ## Non-decorator example:
 * ```
 * class ContextProvider extends LitElement {
 *   @property() contextKey1 = 'value';
 *   @property() contextKey2 = 1;
 *
 *   protected render() {
 *     html`
 *       <slot></slot>
 *     `;
 *   }
 * }
 *
 * customElement('context-provider')(
 *   // Specify which context keys to consume, if the corresponding properties
 *   // don't exist in the class, a compilation error will be thrown.
 *   provideContext('contextKey1')(
 *     provideContext('contextKey2')(
 *       ContextProvider
 *     ),
 *   ),
 * );
 *
 * class ContextConsumer extends LitElement {
 *   @property() contextKey1 = '';
 *
 *   protected render() {
 *     html`${this.contextKey1}`;
 *   }
 * }
 *
 * customElement('context-consumer')(
 *   // Specify which context keys to consume, if the corresponding properties
 *   // don't exist in the class, a compilation error will be thrown.
 *   consumeContext('contextKey1')(ContextConsumer)
 * );
 *
 * class Page extends LitElement {
 *   // renders 'page-value' on screen.
 *   protected render() {
 *     return html`
 *       <context-provider .contextKey1=${'page-value'}>
 *         <context-consumer>
 *         </context-consumer>
 *       </context-provider>
 *     `;
 *   }
 * }
 *
 * customElement('page')(Page)
 * ```
 *
 * ## Type-checked Property Example
 * ```
 * // Create typed decorators to be shared across your app.
 * const provideKey1 = provideContext<'contextKey1', string>('contextKey1');
 * const consumeKey1 = consumeContext<'contextKey1', string>('contextKey1');
 * const provideKey2 = provideContext<'contextKey2', number>('contextKey2');
 * const consumeKey2 = consumeContext<'contextKey2', number>('contextKey2');
 *
 * @customElement('context-provider')
 * @provideKey1
 * @provideKey2
 * class ContextProvider extends LitElement {
 *   // Properties are type checked.
 *   @property() contextKey1 = 'value';
 *   @property() contextKey2 = 1;
 *
 *   protected render() {
 *     html`
 *       <slot></slot>
 *     `;
 *   }
 * }
 *
 * @customElement('context-consumer')
 * @consumeKey1
 * @consumeKey2
 * class ContextConsumer extends LitElement {
 *   // Properties are type checked.
 *   @property() contextKey1 = '';
 *   @property() contextKey2 = 1;
 *
 *   protected render() {
 *     html`${this.contextKey1}`;
 *   }
 * }
 *
 * @customElement('page')
 * class Page extends LitElement {
 *   // renders 'page-value' on screen.
 *   protected render() {
 *     return html`
 *       <context-provider .contextKey1=${'page-value'}>
 *         <context-consumer>
 *         </context-consumer>
 *       </context-provider>
 *     `;
 *   }
 * }
 * ```
 */

import { LitElement } from 'lit-element';

interface ContextEventDetail<Ctx, T extends LitElement & Ctx = LitElement & Ctx> {
  element: T;
  addDisconnectedEventCB(cb: () => void): void;
}
type ContextEvent<Ctx, T extends LitElement & Ctx = LitElement & Ctx> = CustomEvent<ContextEventDetail<Ctx, T>>;

/**
 * Builds a contextProviderMixin, which is a mixin function that takes a
 * LitElement constructor and mixin ContextProvider's behavior.
 * See @fileoverview for examples.
 *
 * ### ContextProvider
 * Conceptually, ContextProvider is an element that can override the context
 * for all of its descendants.
 *
 * When connected to DOM:
 *  * Provides or overrides the context to be observed by ContextConsumers in
 *  descendant nodes.
 *  * Notifies context observers when a context marked with @property() is
 *    updated.
 *
 * When disconnected from DOM:
 *  * Stops providing or overriding the context.
 */
export function provideContext<K extends string, Ctx>(contextKey: K) {
  // The mixin target class (cls) needs to implement Record<K, Ctx>.
  // i.e Ctx must be assignable to the T[K].
  return function providerMixin<T extends LitElement & Record<K, Ctx>, C extends Constructor<T>>(cls: C) {
    // Create new symbols every time we apply the mixin so applying the mixin
    // multiple times won't override the property.
    const consumersSymbol = Symbol('consumers');
    const onSubscribeContextSymbol = Symbol('onSubscribeContext');

    /**
     * Updates a consumer of the given context key with the new context
     * value.
     */
    function updateConsumer(consumer: T, newValue: T[K]) {
      const oldValue = consumer[contextKey];
      if (newValue === oldValue) {
        return;
      }
      consumer[contextKey] = newValue;
    }

    // TypeScript doesn't allow type parameter in extends or implements
    // position. Cast to Constructor<LitElement> to stop tsc complaining.
    class Provider extends (cls as Constructor<LitElement>) {
      [consumersSymbol] = new Set<T>();

      /**
       * Adds the context consumer to the observer list and updates the
       * consumer with the current context immediately.
       */
      [onSubscribeContextSymbol] = (event: ContextEvent<Record<K, Ctx>, T>) => {
        const consumer = event.detail.element;
        this[consumersSymbol].add(consumer);
        event.detail.addDisconnectedEventCB(() => this[consumersSymbol].delete(consumer));
        const newValue = ((this as LitElement) as T)[contextKey];
        this[consumersSymbol].add(consumer);
        updateConsumer(consumer, newValue);
        event.stopImmediatePropagation();
      };

      protected updated(changedProperties: Map<K, Ctx>) {
        super.updated(changedProperties);
        if (!changedProperties.has(contextKey)) {
          return;
        }

        const newValue = ((this as LitElement) as T)[contextKey];
        for (const consumer of this[consumersSymbol]) {
          updateConsumer(consumer, newValue);
        }
      }

      connectedCallback() {
        super.connectedCallback();
        this.addEventListener(`milo-subscribe-context-${contextKey}`, this[onSubscribeContextSymbol] as EventListener);
      }

      disconnectedCallback() {
        this.removeEventListener(
          `milo-subscribe-context-${contextKey}`,
          this[onSubscribeContextSymbol] as EventListener
        );
        this[consumersSymbol] = new Set();
        super.disconnectedCallback();
      }
    }

    // Recover the type information that was lost in the down-casting above.
    return (Provider as Constructor<LitElement>) as C;
  };
}

/**
 * Builds a contextConsumerMixin, which is a mixin function that takes a
 * LitElement constructor and mixin ContextConsumer's behavior.
 * See @fileoverview for examples.
 *
 * ### ContextConsumer
 * Conceptually, ContextConsumer is an element that can subscribe to changes
 * in the context, which is provided/overridden by its ancestor ContextProvider.
 *
 * When connected to DOM:
 *  * Subscribes to the required context keys from the closest ancestor
 *    ContextProvider nodes that provides the context keys.
 *  * Contexts are immediately updated on subscribe.
 *
 * When disconnected from DOM:
 *  * Unsubscribes from all the contextProviders.
 */
export function consumeContext<K extends string, Ctx>(contextKey: K) {
  // The mixin target class (cls) needs to implement Record<K, Ctx>.
  // i.e Ctx must be assignable to the T[K].
  return function consumerMixin<T extends LitElement & Record<K, Ctx>, C extends Constructor<T>>(cls: C) {
    // Create new symbols every time we apply the mixin so applying the mixin
    // multiple times won't override the property.
    const disconnectedEventCBsSymbol = Symbol('disconnectedEventCBs');

    // TypeScript doesn't allow type parameter in extends or implements
    // position. Cast to Constructor<LitElement> to stop tsc complaining.
    class Consumer extends (cls as Constructor<LitElement>) {
      [disconnectedEventCBsSymbol]: Array<() => void> = [];

      connectedCallback() {
        this.dispatchEvent(
          new CustomEvent(`milo-subscribe-context-${contextKey}`, {
            detail: {
              element: (this as LitElement) as T,
              // We need to register callback via subscribe event because
              // dispatching events in disconnectedCallback is no-op.
              addDisconnectedEventCB: (cb: () => void) => {
                this[disconnectedEventCBsSymbol].push(cb);
              },
            },
            bubbles: true,
            cancelable: true,
            composed: true,
          }) as ContextEvent<Record<K, Ctx>, T>
        );
        super.connectedCallback();
      }

      disconnectedCallback() {
        super.disconnectedCallback();
        for (const cb of this[disconnectedEventCBsSymbol].reverse()) {
          cb();
        }
        this[disconnectedEventCBsSymbol] = [];
      }
    }
    // Recover the type information that lost in the down-casting above.
    return (Consumer as Constructor<LitElement>) as C;
  };
}
