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
 * context provider mixin and context subscriber mixin.
 *
 * # Example
 * ```
 * interface Context {
 *  contextKey: string;
 * }
 *
 * const DEFAULT_CONTEXT {
 *   contextKey: 'defaultValue';
 * }
 *
 * @customElement('context-provider')
 * class ContextProvider extends contextProviderMixinBuilder(DEFAULT_CONTEXT)(LitElement) {
 *   @property()
 *   contextKey = 'value';
 *
 *   protected render() {
 *     html`
 *       <slot></slot>
 *     `;
 *   }
 * }
 *
 * @customElement('context-subscriber')
 * class ContextSubscriber extends contextSubscriberMixinBuilder<Context>(['contextKey'])(LitElement) {
 *   @property()
 *   contextValue = '';
 *
 *   protected render() {
 *     html`${this.contextValue}`;
 *   }
 * }
 *
 * @customElement('page)
 * class Page extends LitElement {
 *   // renders 'page-value' on screen.
 *   protected render() {
 *     return html`
 *       <context-provider .contextKey=${'page-value'}>
 *         <context-subscriber>
 *         </context-subscriber>
 *       </context-provider>
 *     `;
 *   }
 * }
 * ```
 */

import { LitElement } from 'lit-element';

interface ContextEventDetail<Ctx> {
  key: keyof Ctx;
  element: ContextSubscriber<Ctx>;
}
type ContextEvent<Ctx> = CustomEvent<ContextEventDetail<Ctx>>;

type ContextSubscriber<Ctx> = Partial<Ctx> & LitElement;

/**
 * Builds a contextProviderMixin, which is a mixin function that takes a
 * LitElement constructor and mixin ContextProvider's behavior.
 *
 * ### ContextProvider
 *  When connected:
 *  * Provides contexts to be observed by ContextConsumer.
 *  * Notifies context observers when a context marked with @property() is
 *    updated.
 *
 * @param providedContexts an object, whose property keys are context keys to be
 *    provided and property values are default values of the corresponding
 *    context keys.
 */
// Use a currying function to be consistent with buildContextSubscriberMixin.
export function buildContextProviderMixin<Ctx>(providedContexts: Ctx) {
  const keys = Object.keys(providedContexts) as Array<keyof Ctx>;

  return function providerMixin<T extends LitElement>(cls: Constructor<T, []>) {
    // TypeScript doesn't allow type parameter in extends or implements
    // position. Cast to Constructor<LitElement> to stop tsc complaining.
    class Provider extends (cls as Constructor<LitElement, []>) {
      private readonly contextSubscribers = new Map<keyof Ctx, Set<ContextSubscriber<Ctx>>>(keys.map((key) => [key, new Set()]));

      constructor() {
        super();
        Object.assign(this, providedContexts);
      }

      protected updated(changedProperties: Map<keyof Ctx, Ctx[keyof Ctx]>) {
        super.updated(changedProperties);

        for (const key of changedProperties.keys()) {
          const subscribers = this.contextSubscribers.get(key);
          if (!subscribers) {
            continue;
          }
          const newValue = (this as ContextSubscriber<Ctx>)[key];
          for (const subscriber of subscribers) {
            this.updateSubscriber(subscriber, key, newValue);
          }
        }
      }

      /**
       * Updates a subscriber of the given context key with the new context
       * value.
       */
      protected updateSubscriber(subscriber: ContextSubscriber<Ctx>, key: keyof Ctx, newValue: ContextSubscriber<Ctx>[keyof Ctx]) {
        const oldValue = subscriber[key];
        if (newValue === oldValue) {
          return;
        }
        subscriber[key] = newValue;
        subscriber.requestUpdate(key, oldValue);
      }

      connectedCallback() {
        super.connectedCallback();
        this.addEventListener('tr-subscribe-context', this.onSubscribeContext as EventListener);
        this.addEventListener('tr-unsubscribe-context', this.onUnsubscribeContext as EventListener);
      }
      disconnectedCallback() {
        super.disconnectedCallback();
        this.removeEventListener('tr-subscribe-context', this.onSubscribeContext as EventListener);
        this.removeEventListener('tr-unsubscribe-context', this.onUnsubscribeContext as EventListener);
      }

      /**
       * If `this` provides the context the subscriber wants to subscribe to,
       * add the context subscriber to the observer list and update the
       * subscriber with the current context immediately.
       * Otherwise, let the next event handler (if any) handles it.
       */
      private onSubscribeContext = (event: ContextEvent<Partial<Ctx>>) => {
        const subscribers = this.contextSubscribers.get(event.detail.key);
        if (subscribers) {
          const subscriber = event.detail.element;
          const key = event.detail.key;
          const newValue = (this as ContextSubscriber<Ctx>)[event.detail.key];
          subscribers.add(subscriber);
          this.updateSubscriber(subscriber, key, newValue);
          event.stopImmediatePropagation();
        }
      }

      /**
       * If `this` provides the context the subscriber wants to subscribe, remove
       * the context subscriber from the observer list. Otherwise, let the next
       * event handler (if any) handles it.
       */
      private onUnsubscribeContext = (event: ContextEvent<Partial<Ctx>>) => {
        const subscribers = this.contextSubscribers.get(event.detail.key);
        if (subscribers) {
          subscribers.delete(event.detail.element);
          event.stopImmediatePropagation();
        }
      }
    } 
    // Recover the type information that lost in the down-casting above.
    return Provider as Constructor<Partial<Ctx> & Provider & Partial<T>> as Constructor<Ctx & Provider & T>;
  };
}


/**
 * Builds a contextConsumerMixin, which is a mixin function that takes a
 * LitElement constructor and mixin ContextConsumer's behavior.
 *
 * ### ContextConsumer
 *  * When connected to DOM, subscribes to the required contexts from the
 *    closest ancestor ContextProvider node that provides the contexts.
 *  * Contexts are immediately updated on subscribe.
 *  * When disconnected from DOM, unsubscribes from the subscribed
 *    ContextProviders.
 *
 * @param requiredContexts context keys to subscribe to.
 */
// TypeScript doesn't support partial type inference.
// (e.g. partiallyInferredFunctionCall<_, number | undefined>(1, 2))
// Use a currying function so caller can rely on type inference for
// subscriberMixin<T extends LitElement>(cls: Constructor<T>).
export function buildContextSubscriberMixin<Ctx>(requiredContexts: Array<keyof Ctx>) {
  return function subscriberMixin<T extends LitElement>(cls: Constructor<T, []>) {
    // TypeScript doesn't allow type parameter in extends or implements
    // position. Cast to Constructor<LitElement> to stop tsc complaining.
    class Consumer extends (cls as Constructor<LitElement, []>) {
      connectedCallback() {
        super.connectedCallback();
        for (const key of requiredContexts) {
          this.dispatchEvent(new CustomEvent('tr-subscribe-context', {
            detail: {key, element: this},
            bubbles: true,
            cancelable: true,
            composed: true,
          }) as ContextEvent<Ctx>);
        }
      }

      disconnectedCallback() {
        super.disconnectedCallback();
        for (const key of requiredContexts) {
          this.dispatchEvent(new CustomEvent('tr-unsubscribe-context', {
            detail: {key, element: this},
            bubbles: true,
            cancelable: true,
            composed: true,
          }) as ContextEvent<Ctx>);
        }
      }
    }
    // Recover the type information that lost in the down-casting above.
    return Consumer as Constructor<Partial<Ctx> & Consumer & Partial<T>> as Constructor<Ctx & Consumer & T>;
  };
}
