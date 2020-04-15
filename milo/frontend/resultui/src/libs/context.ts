/* Copyright 2020 The LUCI Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 * @fileoverview This file contains helper functions for constructing
 * context provider mixin and context consumer mixin.
 * The design is heavily inspired by
 * [React Context](https://reactjs.org/docs/context.html)
 *
 * # Example
 * ```
 * // Declare interface for Context, usually shared cross the entire app.
 * interface Context {
 *   contextKey1: string;
 *   contextKey2: number;
 * }
 *
 * // Initialize context provider and consumer mixin with Context, usually
 * // shared across the entire app.
 * const provideContext = contextProviderMixinBuilder<Context>();
 * const consumeContext = contextConsumerMixinBuilder<Context>();
 * ```
 *
 * ## Decorator Example
 * ```
 * @customElement('context-provider')
 * // Specify which context keys to provide. Parameters are type checked.
 * // Passing 'contextKey3' will cause compilation error.
 * @provideContext('contextKey1', 'contextKey2')
 * class ContextProvider extends LitElement {
 *   // contextKey1 is type checked, it must be of type string.
 *   @property() contextKey1 = 'value';
 *
 *   // contextKey2 is type checked, it must be of type number.
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
 * // Specify which context keys to consume. Parameters are type checked.
 * // Passing 'contextKey3' will cause compilation error.
 * @consumeContext('contextKey')
 * class ContextConsumer extends LitElement {
 *   // contextKey1 is type checked, it must be of type string.
 *   @property() contextKey1 = '';
 *
 *   // contextKey2 is NOT type checked,
 *   // because this element is not consuming contextKey2.
 *   @property() contextKey2 = '';
 *
 *   protected render() {
 *     html`${this.contextValue}`;
 *   }
 * }
 *
 * @customElement('page')
 * class Page extends LitElement {
 *   // renders 'page-value' on screen.
 *   protected render() {
 *     return html`
 *       <context-provider .contextKey=${'page-value'}>
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
 *   // contextKey1 is type checked, it must be of type string.
 *   @property() contextKey1 = 'value';
 *
 *   // contextKey2 is type checked, it must be of type number.
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
 *   // Specify which context keys to provide. Parameters are type checked.
 *   // Passing 'contextKey3' will cause compilation error.
 *   provideContext('contextKey1', 'contextKey2')(ContextProvider)
 * );
 *
 * class ContextConsumer extends LitElement {
 *   // contextKey1 is type checked, it must be of type string.
 *   @property() contextKey1 = '';
 *
 *   // contextKey2 is NOT type checked,
 *   // because this element is not consuming contextKey2.
 *   @property() contextKey2 = '';
 *
 *   protected render() {
 *     html`${this.contextValue}`;
 *   }
 * }
 *
 * customElement('context-consumer')(
 *   // Specify which context keys to consume. Parameters are type checked.
 *   // Passing 'contextKey3' will cause compilation error.
 *   consumeContext('contextKey1')(ContextConsumer)
 * );
 *
 * class Page extends LitElement {
 *   // renders 'page-value' on screen.
 *   protected render() {
 *     return html`
 *       <context-provider .contextKey=${'page-value'}>
 *         <context-consumer>
 *         </context-consumer>
 *       </context-provider>
 *     `;
 *   }
 * }
 *
 * customElement('page')(Page)
 * ```
 */

import { LitElement } from 'lit-element';

interface ContextEventDetail<Ctx, T extends LitElement & Ctx = LitElement & Ctx> {
  element: T;
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
export function contextProviderMixinBuilder<Ctx>() {
  return function provideKeys<K extends keyof Ctx>(...providedContextKeys: K[]) {
    // The mixin target class (cls) needs to implement Pick<Ctx, K>.
    // i.e for each property name in observedContextKeys, the property in T
    // must be assignable to the property of the same name in Ctx.
    return function providerMixin<T extends LitElement & Pick<Ctx, K>>(cls: Constructor<T>) {
      // TypeScript doesn't allow type parameter in extends or implements
      // position. Cast to Constructor<LitElement> to stop tsc complaining.
      class Provider extends (cls as Constructor<LitElement>) {
        private readonly contextConsumers = new Map<K, Set<T>>(providedContextKeys.map((key) => [key, new Set()]));

        protected updated(changedProperties: Map<K, Ctx[K]>) {
          super.updated(changedProperties);

          for (const key of changedProperties.keys()) {
            const consumers = this.contextConsumers.get(key);
            if (!consumers) {
              continue;
            }
            const newValue = (this as LitElement as T)[key];
            for (const consumer of consumers) {
              this.updateConsumer(consumer, key, newValue);
            }
          }
        }

        /**
         * Updates a consumer of the given context key with the new context
         * value.
         */
        private updateConsumer(consumer: T, key: K, newValue: T[K]) {
          const oldValue = consumer[key];
          if (newValue === oldValue) {
            return;
          }
          consumer[key] = newValue;
          consumer.requestUpdate(key, oldValue);
        }

        connectedCallback() {
          super.connectedCallback();
          for (const key of providedContextKeys) {
            this.addEventListener(`tr-subscribe-context-${key}`, this.onSubscribeContext as EventListener);
            this.addEventListener(`tr-unsubscribe-context-${key}`, this.onUnsubscribeContext as EventListener);
          }
        }
        disconnectedCallback() {
          super.disconnectedCallback();
          for (const key of providedContextKeys) {
            this.removeEventListener(`tr-subscribe-context-${key}`, this.onSubscribeContext as EventListener);
            this.removeEventListener(`tr-unsubscribe-context-${key}`, this.onUnsubscribeContext as EventListener);
          }
        }

        /**
         * Adds the context consumer to the observer list and updates the
         * consumer with the current context immediately.
         */
        private onSubscribeContext = (event: ContextEvent<Pick<Ctx, K>, T>) => {
          const key = event.type.match(/tr-subscribe-context-(.*)/)![1] as K;
          const consumers = this.contextConsumers.get(key)!;
          const consumer = event.detail.element;
          const newValue = (this as LitElement as T)[key];
          consumers.add(consumer);
          this.updateConsumer(consumer, key, newValue);
          event.stopImmediatePropagation();
        }

        /**
         * Removes the context consumer from the observer list.
         */
        private onUnsubscribeContext = (event: ContextEvent<Pick<Ctx, K>, T>) => {
          const key = event.type.match(/tr-unsubscribe-context-(.*)/)![1] as K;
          const consumers = this.contextConsumers.get(key)!;
          consumers.delete(event.detail.element);
          event.stopImmediatePropagation();
        }
      }

      // Recover the type information that was lost in the down-casting above.
      return Provider as Constructor<LitElement> as Constructor<T>;
    };
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
export function contextConsumerMixinBuilder<Ctx>() {
  return function consumeKeys<K extends keyof Ctx>(...observedContextKeys: K[]) {
    // The mixin target class (cls) needs to implement Pick<Ctx, K>.
    // i.e for each property name in observedContextKeys, the property in T
    // must be assignable to the property of the same name in Ctx.
    return function consumerMixin<T extends LitElement & Pick<Ctx, K>>(cls: Constructor<T>) {
      // TypeScript doesn't allow type parameter in extends or implements
      // position. Cast to Constructor<LitElement> to stop tsc complaining.
      class Consumer extends (cls as Constructor<LitElement>) {
        connectedCallback() {
          super.connectedCallback();
          this.emitEvents('subscribe');
        }

        disconnectedCallback() {
          super.disconnectedCallback();
          this.emitEvents('unsubscribe');
        }

        private emitEvents(type: 'subscribe' | 'unsubscribe') {
          for (const key of observedContextKeys) {
            this.dispatchEvent(new CustomEvent(`tr-${type}-context-${key}`, {
              detail: {element: this as LitElement as T},
              bubbles: true,
              cancelable: true,
              composed: true,
            }) as ContextEvent<Pick<Ctx, K>, T>);
          }
        }
      }
      // Recover the type information that lost in the down-casting above.
      return Consumer as Constructor<LitElement> as Constructor<T>;
    };
  };
}
