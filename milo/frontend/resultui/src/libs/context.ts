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

interface ProviderProperties<T> {
  consumers: Set<T>;
  onSubscribeContext?: EventListener;
}

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
    // Use a weak Map to store private properties, so overriding those
    // properties won't break the class.
    // This can happen when applying the decorator multiple times.
    const providerPropsWeakMap = new WeakMap<Provider, ProviderProperties<T>>();

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
      constructor() {
        super();
        providerPropsWeakMap.set(this, {consumers: new Set()});
      }

      protected updated(changedProperties: Map<K, Ctx>) {
        super.updated(changedProperties);
        if (!changedProperties.has(contextKey)) {
          return;
        }

        const consumers = providerPropsWeakMap.get(this)!.consumers;
        const newValue = (this as LitElement as T)[contextKey];
        for (const consumer of consumers) {
          updateConsumer(consumer, newValue);
        }
      }

      connectedCallback() {
        const props = providerPropsWeakMap.get(this)!;

        /**
         * Adds the context consumer to the observer list and updates the
         * consumer with the current context immediately.
         */
        props.onSubscribeContext = ((event: ContextEvent<Record<K, Ctx>, T>) => {
          const consumers = props.consumers;
          const consumer = event.detail.element;
          consumers.add(consumer);
          event.detail.addDisconnectedEventCB(() => consumers.delete(consumer));
          const newValue = (this as LitElement as T)[contextKey];
          consumers.add(consumer);
          updateConsumer(consumer, newValue);
          event.stopImmediatePropagation();
        }) as EventListener;

        this.addEventListener(`milo-subscribe-context-${contextKey}`, props.onSubscribeContext);
        super.connectedCallback();
      }

      disconnectedCallback() {
        const props = providerPropsWeakMap.get(this)!;
        this.removeEventListener(`milo-subscribe-context-${contextKey}`, props.onSubscribeContext!);
        super.disconnectedCallback();
      }
    }

    // Recover the type information that was lost in the down-casting above.
    return Provider as Constructor<LitElement> as C;
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
    const disconnectedEventCBs: Array<() => void> = [];

    // TypeScript doesn't allow type parameter in extends or implements
    // position. Cast to Constructor<LitElement> to stop tsc complaining.
    class Consumer extends (cls as Constructor<LitElement>) {
      connectedCallback() {
        this.dispatchEvent(new CustomEvent(`milo-subscribe-context-${contextKey}`, {
          detail: {
            element: this as LitElement as T,
            // We need to register callback via subscribe event because
            // dispatching events in disconnectedCallback is no-op.
            addDisconnectedEventCB(cb: () => void) {
              disconnectedEventCBs.push(cb);
            },
          },
          bubbles: true,
          cancelable: true,
          composed: true,
        }) as ContextEvent<Record<K, Ctx>, T>);
        super.connectedCallback();
      }

      disconnectedCallback() {
        for (const cb of disconnectedEventCBs) {
          cb();
        }
        super.disconnectedCallback();
      }
    }
    // Recover the type information that lost in the down-casting above.
    return Consumer as Constructor<LitElement> as C;
  };
}
