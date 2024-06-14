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
 * @fileoverview This file contains decorators and decorator builders to
 * help passing properties to descendants deep in the tree without passing
 * properties explicitly through every layer.
 *
 * The design is heavily inspired by
 * [React Context](https://reactjs.org/docs/context.html)
 *
 * ## Decorator Example
 * ```
 * const [provideString, consumeString] = createContextLink<string>();
 * const [provideNumber, consumeNumber] = createContextLink<number>();
 *
 * @customElement('context-provider')
 * // Context providers needs to be denoted with @provider
 * @provider
 * class ContextProvider extends MobxLitElement {
 *   // Context bind properties in a context provider HAVE TO be decorated with
 *   // @property(). Otherwise changes won't be propagated.
 *   @property()
 *   @provideString()
 *   stringKey = 'value';
 *
 *   @property()
 *   // When there are multiple properties providing the same context, the last
 *   // on is used.
 *   @provideNumber()
 *   ignoredNumberKey = 1;
 *
 *   @property()
 *   // provideNumber type checks that the property is a number.
 *   @provideNumber()
 *   numberKey = 1;
 *
 *   protected render() {
 *     html`
 *       <slot></slot>
 *     `;
 *   }
 * }
 *
 * @customElement('context-consumer')
 * // Context consumers needs to be denoted with @consumer
 * @consumer
 * class ContextConsumer extends MobxLitElement {
 *   @property()
 *   @consumeString()
 *   // The property key don't have to be the same as the property key used in
 *   // the context provider
 *   aDifferentStringKey = '';
 *
 *   // Context bind properties in a context consumer DOES NOT have to be
 *   // decorated with @property().
 *   @consumeNumber()
 *   // Context bind properties in a context consumer can be optional.
 *   numberKey?: number;
 *
 *   protected render() {
 *     html`${this.aDifferentStringKey}`;
 *   }
 * }
 *
 * @customElement('page')
 * class Page extends MobxLitElement {
 *   // renders 'page-value' on screen.
 *   protected render() {
 *     return html`
 *       <context-provider .stringKey=${'page-value'}>
 *         <context-consumer>
 *         </context-consumer>
 *       </context-provider>
 *     `;
 *   }
 * }
 * ```
 */

import 'reflect-metadata';
import { MobxLitElement } from '@adobe/lit-mobx';
import { PropertyValues } from 'lit';

import { Constructor } from '@/generic_libs/types';

interface ContextEventDetail {
  onCtxUpdate(newCtxValue: unknown): void;
  addDisconnectedEventCB(cb: () => void): void;
}
type ContextEvent = CustomEvent<ContextEventDetail>;

export interface CtxProviderOption {
  /**
   * This flag enables the provider to provide the context to all consumers
   * rather than only to the provider's descendants. It can also improve
   * performance because context consumers no longer need to dispatch events in
   * a DoM tree (only if there's no registered local providers for the same
   * context).
   *
   * Caveats:
   *  1. There can be at most one global provider for this context.
   *  2. The provider should be connected before any consumer is connected.
   *  3. The provider should not be disconnected unless all consumers are
   * either disconnected or about to be disconnected.
   *
   * Defaults to false.
   */
  readonly global: boolean;
}

// Use a Map to so there can be only one property mapped to the context in a
// provider.
type ProviderContextMeta = Map<
  string,
  [string | number | symbol, CtxProviderOption]
>;

// Use an array so there can be multiple properties mapped to the same context
// in a consumer.
type ConsumerContextMeta = Array<
  [eventType: string, propKey: string | number | symbol]
>;

const providerMetaSymbol = Symbol('provider');
const consumerMetaSymbol = Symbol('consumer');

const globalContextProviders = new Map<string, EventListener>();
const localContextProviderCounter = new Map<string, number>();

/**
 * Marks a component as a context provider.
 *
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
 *
 * See @fileoverview for examples.
 */
export function provider<Cls extends Constructor<MobxLitElement>>(cls: Cls) {
  // Create new symbols every time we apply the mixin so applying the mixin
  // multiple times won't override the property.
  const onCtxUpdatesSymbol = Symbol('onCtxUpdates');
  const disconnectedCBsSymbol = Symbol('disconnectedCBs');
  const isFirstUpdatedSymbol = Symbol('isFirstUpdated');

  const meta: ProviderContextMeta =
    Reflect.getMetadata(providerMetaSymbol, cls.prototype) || new Map();
  const eventTypes = [...meta.keys()];

  class Provider extends (cls as Constructor<MobxLitElement>) {
    [onCtxUpdatesSymbol] = new Map<string, Set<(newCtx: unknown) => void>>(
      eventTypes.map((eventType) => [eventType, new Set()]),
    );
    [disconnectedCBsSymbol]: Array<() => void> = [];
    [isFirstUpdatedSymbol] = true;

    protected updated(changedProperties: PropertyValues) {
      super.updated(changedProperties);

      // Subscribe events are fired in this.connectedCallback() and onCtxUpdate
      // callbacks are called immediately in the event handler.
      // this.updated is then called after this.connectedCallback().
      // Since onCtxUpdate callbacks were called in the event handler already,
      // skip this to avoid double calling.
      if (this[isFirstUpdatedSymbol]) {
        this[isFirstUpdatedSymbol] = false;
        return;
      }

      for (const [eventType, [propKey]] of meta) {
        if (!changedProperties.has(propKey)) {
          continue;
        }
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const newValue = (this as any)[propKey];
        const onCtxUpdateSet = this[onCtxUpdatesSymbol].get(eventType)!;
        for (const cb of onCtxUpdateSet) {
          cb(newValue);
        }
      }
    }

    connectedCallback() {
      for (const [eventType, [propKey, opt]] of meta) {
        const onCtxUpdateSet = this[onCtxUpdatesSymbol].get(eventType)!;

        /**
         * Adds the context consumer to the observer list and updates the
         * consumer with the current context immediately.
         */
        const eventCB = ((event: ContextEvent) => {
          const onCtxUpdate = event.detail.onCtxUpdate;
          onCtxUpdateSet.add(onCtxUpdate);
          event.detail.addDisconnectedEventCB(() =>
            onCtxUpdateSet.delete(onCtxUpdate),
          );
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          onCtxUpdate((this as any)[propKey]);
          event.stopImmediatePropagation();
          // Signals that the consumer should not fallback to global context
          // provider.
          event.preventDefault();
        }) as EventListener;

        this.addEventListener(eventType, eventCB);
        this[disconnectedCBsSymbol].push(() =>
          this.removeEventListener(eventType, eventCB),
        );

        if (opt.global) {
          if (globalContextProviders.has(eventType)) {
            // Not much we can do when a global context provider is overridden.
            // eslint-disable-next-line no-console
            console.warn('Overriding a global context provider.');
          }

          globalContextProviders.set(eventType, eventCB);
          this[disconnectedCBsSymbol].push(() => {
            if (globalContextProviders.get(eventType) === eventCB) {
              globalContextProviders.delete(eventType);
            }
          });
        } else {
          localContextProviderCounter.set(
            eventType,
            (localContextProviderCounter.get(eventType) || 0) + 1,
          );
          this[disconnectedCBsSymbol].push(() => {
            localContextProviderCounter.set(
              eventType,
              localContextProviderCounter.get(eventType)! - 1,
            );
          });
        }
      }
      super.connectedCallback();
    }

    disconnectedCallback() {
      super.disconnectedCallback();
      for (const cb of this[disconnectedCBsSymbol].reverse()) {
        cb();
      }
      this[disconnectedCBsSymbol] = [];
    }
  }

  // Recover the type information that was lost in the down-casting above.
  return Provider as Constructor<MobxLitElement> as Cls;
}

/**
 * Marks a component as a context consumer.
 *
 * Conceptually, ContextConsumer is an element that can subscribe to changes
 * in the context, which is provided/overridden by its ancestor ContextProvider.
 *
 * When connected to DOM:
 *  * Subscribes to the required context from the closest ancestor
 *    ContextProvider nodes that provides the context.
 *  * Contexts are immediately updated on subscribe.
 *
 * When disconnected from DOM:
 *  * Unsubscribes from all the contextProviders.
 *
 * See @fileoverview for examples.
 */
export function consumer<Cls extends Constructor<MobxLitElement>>(cls: Cls) {
  // Create new symbols every time we apply the mixin so applying the mixin
  // multiple times won't override the property.
  const disconnectedCBsSymbol = Symbol('disconnectedCBs');

  const meta =
    (Reflect.getMetadata(consumerMetaSymbol, cls.prototype) as
      | ConsumerContextMeta
      | undefined) || [];

  // TypeScript doesn't allow type parameter in extends or implements
  // position. Cast to Constructor<MobxLitElement> to stop tsc complaining.
  class Consumer extends (cls as Constructor<MobxLitElement>) {
    [disconnectedCBsSymbol]: Array<() => void> = [];

    connectedCallback() {
      for (const [eventType, propKey] of meta) {
        const subscribeEvent = new CustomEvent<ContextEventDetail>(eventType, {
          detail: {
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            onCtxUpdate: (newCtx: unknown) => ((this as any)[propKey] = newCtx),
            // We need to register callback via subscribe event because
            // dispatching events in disconnectedCallback is no-op.
            addDisconnectedEventCB: (cb: () => void) => {
              this[disconnectedCBsSymbol].push(cb);
            },
          },
          bubbles: true,
          cancelable: true,
          composed: true,
        });

        const globalHandler = globalContextProviders.get(eventType);
        // Use the global handler directly if no local handlers are registered.
        // This bypasses event dispatch therefore improves performance.
        if (globalHandler && !localContextProviderCounter.get(eventType)) {
          globalHandler(subscribeEvent);
        } else {
          const shouldFallback = this.dispatchEvent(subscribeEvent);
          // Fallback to the global handler if a local handler is not found.
          if (shouldFallback && globalHandler) {
            globalHandler(subscribeEvent);
          }
        }
      }
      super.connectedCallback();
    }

    disconnectedCallback() {
      super.disconnectedCallback();
      for (const cb of this[disconnectedCBsSymbol].reverse()) {
        cb();
      }
      this[disconnectedCBsSymbol] = [];
    }
  }

  // Recover the type information that was lost in the down-casting above.
  return Consumer as Constructor<MobxLitElement> as Cls;
}

const DEFAULT_PROVIDER_OPT: CtxProviderOption = Object.freeze({
  global: false,
});

/**
 * Builds 2 property decorators.
 * The first one can be used mark properties that should be used to set the
 * context.
 * The second one can be used to mark properties that should get their value
 * from the context provided by the closest ancestor components with properties
 * marked with the first decorator.
 *
 * See @fileoverview for examples.
 */
export function createContextLink<Ctx>() {
  const eventType = 'milo-subscribe-context-' + Math.random();

  function provideContext(opt = DEFAULT_PROVIDER_OPT) {
    return function <
      K extends string | number | symbol,
      // Ctx must be assignable to T[K].
      T extends MobxLitElement & Record<K, Ctx>,
    >(target: T, propKey: K) {
      if (!Reflect.hasMetadata(providerMetaSymbol, target)) {
        Reflect.defineMetadata(providerMetaSymbol, new Map(), target);
      }
      const meta = Reflect.getMetadata(
        providerMetaSymbol,
        target,
      ) as ProviderContextMeta;
      meta.set(eventType, [propKey, opt]);
    };
  }

  function consumeContext() {
    return function <
      K extends string | number | symbol,
      V,
      T extends MobxLitElement & Partial<Record<K, V>>,
    >(
      // T[K] must be assignable to Ctx.
      target: Ctx extends T[K] ? T : never,
      propKey: K,
    ) {
      if (!Reflect.hasMetadata(consumerMetaSymbol, target)) {
        Reflect.defineMetadata(consumerMetaSymbol, [], target);
      }
      const meta = Reflect.getMetadata(
        consumerMetaSymbol,
        target,
      ) as ConsumerContextMeta;
      meta.push([eventType, propKey]);
    };
  }

  return [provideContext, consumeContext] as [
    typeof provideContext,
    typeof consumeContext,
  ];
}
