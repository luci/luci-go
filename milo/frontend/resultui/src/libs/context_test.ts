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

import { fixture, fixtureCleanup } from '@open-wc/testing/index-no-side-effects';
import { assert } from 'chai';
import { customElement, html, LitElement, property } from 'lit-element';

import { consumer, createContextLink, provider } from './context';

class Parent {
  prop1 = 1;
}

class Child extends Parent {
  prop2 = 2;
}

class GrandChild extends Child {
  prop3 = 3;
}

const [provideOuterProviderInactiveKey, consumeOuterProviderInactiveKey] = createContextLink<string>();
const [provideOuterProviderKey, consumeOuterProviderKey] = createContextLink<string>();
const [provideProviderKey, consumeProviderKey] = createContextLink<string>();
const [, consumeUnprovidedKey] = createContextLink<string>();
const [provideOuterUnobservedKey] = createContextLink<string>();
const [provideConsumerOptionalPropKey, consumeConsumerOptionalPropKey] = createContextLink<string>();
const [provideSubtypeKey, consumeSubtypeKey] = createContextLink<Child>();
const [provideSelfProvidedKey, consumeSelfProvidedKey] = createContextLink<string>();

@customElement('milo-outer-context-provider-test')
@provider
class OuterContextProvider extends LitElement {
  @provideOuterProviderInactiveKey()
  outerProviderInactiveKey = 'outer_provider-outer_provider_inactive-val0';

  @property()
  @provideOuterProviderKey()
  outerProviderKey = 'outer_provider-outer_provider-val0';

  // The same context is provided by multiple properties, the last one is used.
  @property()
  @provideProviderKey()
  ignoredProviderKey = 'ignored_outer_provider-provider-val0';

  @property()
  @provideProviderKey()
  providerKey = 'outer_provider-provider-val0';

  @property()
  @provideOuterUnobservedKey()
  outerUnobservedKey = 'outer_provider-outer_unobserved-val0';

  @property()
  unprovidedKey = 'outer_provider-unprovided-val0';

  @property()
  @provideConsumerOptionalPropKey()
  consumerOptionalPropKey = 'outer_provider-consumer_optional_pro-val0';

  // // This should not compile because Parent is not assignable to Child.
  // @provideSubtypeKey
  // parent = new Parent();

  @provideSubtypeKey()
  child = new GrandChild();

  // This should compile without warning because GrandChild can be assigned to
  // parent.
  @provideSubtypeKey()
  grandChild = new GrandChild();
}

@customElement('milo-inner-context-provider-test')
@provider
class InnerContextProvider extends LitElement {
  @property()
  @provideProviderKey()
  providerKey = 'inner_provider-provider-val0';
}

@customElement('milo-global-context-provider-test')
@provider
class GlobalContextProvider extends LitElement {
  @property()
  @provideProviderKey({ global: true })
  providerKey = 'global_provider-provider-val0';
}

@customElement('milo-context-consumer-test')
@provider
@consumer
class ContextConsumer extends LitElement {
  @property()
  @consumeOuterProviderInactiveKey()
  outerProviderInactiveKey = 'local-outer_provider_inactive';

  @property()
  @consumeOuterProviderKey()
  outerProviderKey = 'local-output_provider';

  @consumeProviderKey()
  providerKey = 'local-provider';

  // Multiple props can map to the same context.
  @consumeProviderKey()
  providerKeyWithAnotherName = 'local-provider';

  @property()
  @consumeUnprovidedKey()
  unprovidedKey = 'local-unprovided';

  @property()
  outerUnobservedKey = 'local-unobserved';

  // This should compile without warnings.
  @property()
  @consumeConsumerOptionalPropKey()
  consumerOptionalPropKey?: string;

  // This should compile without warning because Child is assignable to Parent.
  @consumeSubtypeKey()
  parent = new Parent();

  // // This should not compile because Child is not assignable to GrandChild.
  // @consumeSubtypeKey
  // grandChild = new GrandChild();

  @consumeSubtypeKey()
  child = new Child();

  @property()
  @provideSelfProvidedKey()
  selfProvided = 'local-self_provided';

  @property()
  @consumeSelfProvidedKey()
  selfProvided2 = '';

  // Help testing that the property is not set too many times, particularly
  // when the element is first rendered.
  setterCallCount = 0;
  @consumeProviderKey()
  set providerKeySetter(_newVal: string) {
    this.setterCallCount++;
  }
}

@customElement('milo-context-consumer-wrapper-test')
export class ContextConsumerWrapper extends LitElement {
  protected render() {
    return html` <milo-context-consumer-test></milo-context-consumer-test> `;
  }
}

// TODO(weiweilin): test what happens when ContextProvider is disconnected from
// DOM then reconnected to DOM.
describe('context', () => {
  describe('ContextProvider', () => {
    beforeEach(fixtureCleanup);
    after(fixtureCleanup);

    it('should provide context to descendent context consumers', async () => {
      const outerProvider = await fixture<OuterContextProvider>(html`
        <milo-outer-context-provider-test>
          <milo-inner-context-provider-test>
            <milo-context-consumer-test id="inner-consumer"> </milo-context-consumer-test>
          </milo-inner-context-provider-test>
          <milo-context-consumer-test id="outer-consumer"> </milo-context-consumer-test>
          <milo-outer-context-provider> </milo-outer-context-provider
        ></milo-outer-context-provider-test>
      `);

      const innerProvider = outerProvider.querySelector('milo-inner-context-provider-test')!.shadowRoot!
        .host as InnerContextProvider;
      const outerConsumer = outerProvider.querySelector('#outer-consumer')!.shadowRoot!.host as ContextConsumer;
      const innerConsumer = outerProvider.querySelector('#inner-consumer')!.shadowRoot!.host as ContextConsumer;

      assert.strictEqual(outerConsumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(outerConsumer.outerProviderKey, 'outer_provider-outer_provider-val0');
      assert.strictEqual(outerConsumer.providerKey, 'outer_provider-provider-val0');
      assert.strictEqual(outerConsumer.providerKeyWithAnotherName, 'outer_provider-provider-val0');
      assert.strictEqual(outerConsumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(outerConsumer.unprovidedKey, 'local-unprovided');
      assert.strictEqual(outerConsumer.selfProvided2, 'local-self_provided');
      assert.strictEqual(outerConsumer.setterCallCount, 1);

      assert.strictEqual(innerConsumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(innerConsumer.outerProviderKey, 'outer_provider-outer_provider-val0');
      assert.strictEqual(innerConsumer.providerKey, 'inner_provider-provider-val0');
      assert.strictEqual(innerConsumer.providerKeyWithAnotherName, 'inner_provider-provider-val0');
      assert.strictEqual(innerConsumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(innerConsumer.unprovidedKey, 'local-unprovided');
      assert.strictEqual(innerConsumer.setterCallCount, 1);

      // Update outer provider.
      outerProvider.outerProviderInactiveKey = 'outer_provider-outer_provider_inactive-val1';
      outerProvider.outerProviderKey = 'outer_provider-outer_provider-val1';
      outerProvider.providerKey = 'outer_provider-provider-val1';
      outerProvider.outerUnobservedKey = 'outer_provider-unobserved_val1';
      outerProvider.unprovidedKey = 'outer_provider-local_val1';
      await outerProvider.updateComplete;

      // outerConsumer updated.
      assert.strictEqual(outerConsumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(outerConsumer.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(outerConsumer.providerKey, 'outer_provider-provider-val1');
      assert.strictEqual(outerConsumer.providerKeyWithAnotherName, 'outer_provider-provider-val1');
      assert.strictEqual(outerConsumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(outerConsumer.unprovidedKey, 'local-unprovided');
      assert.strictEqual(outerConsumer.setterCallCount, 2);

      // innerConsumer.providerKey unchanged, other properties updated.
      assert.strictEqual(innerConsumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(innerConsumer.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(innerConsumer.providerKey, 'inner_provider-provider-val0');
      assert.strictEqual(innerConsumer.providerKeyWithAnotherName, 'inner_provider-provider-val0');
      assert.strictEqual(innerConsumer.unprovidedKey, 'local-unprovided');
      assert.strictEqual(innerConsumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(innerConsumer.setterCallCount, 1);

      // Update inner provider.
      innerProvider.providerKey = 'inner_provider-provider-val1';
      await innerProvider.updateComplete;

      // outerConsumer unchanged.
      assert.strictEqual(outerConsumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(outerConsumer.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(outerConsumer.providerKey, 'outer_provider-provider-val1');
      assert.strictEqual(outerConsumer.providerKeyWithAnotherName, 'outer_provider-provider-val1');
      assert.strictEqual(outerConsumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(outerConsumer.unprovidedKey, 'local-unprovided');
      assert.strictEqual(outerConsumer.setterCallCount, 2);

      // innerConsumer.providerKey updated, other properties unchanged.
      assert.strictEqual(innerConsumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(innerConsumer.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(innerConsumer.providerKey, 'inner_provider-provider-val1');
      assert.strictEqual(innerConsumer.providerKeyWithAnotherName, 'inner_provider-provider-val1');
      assert.strictEqual(innerConsumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(innerConsumer.unprovidedKey, 'local-unprovided');
      assert.strictEqual(innerConsumer.setterCallCount, 2);
    });

    it('should provide context to any context consumers if global is set to true', async () => {
      const rootEle = await fixture<HTMLDivElement>(html`
        <div>
          <milo-outer-context-provider-test></milo-outer-context-provider-test>
          <milo-global-context-provider-test></milo-global-context-provider-test>
          <milo-context-consumer-wrapper-test></milo-context-consumer-wrapper-test>
        </div>
      `);
      const globalProvider = rootEle.querySelector('milo-global-context-provider-test')!.shadowRoot!
        .host as GlobalContextProvider;
      const consumer = rootEle
        .querySelector('milo-context-consumer-wrapper-test')!
        .shadowRoot!.querySelector('milo-context-consumer-test')!.shadowRoot!.host as ContextConsumer;

      assert.strictEqual(consumer.providerKey, 'global_provider-provider-val0');

      // Update global provider.
      globalProvider.providerKey = 'global_provider-provider-val1';
      await globalProvider.updateComplete;

      // consumer.providerKey updated
      assert.strictEqual(consumer.providerKey, 'global_provider-provider-val1');
    });

    it('global context should not override local context', async () => {
      const rootEle = await fixture<HTMLDivElement>(html`
        <div>
          <milo-global-context-provider-test></milo-global-context-provider-test>
          <milo-outer-context-provider-test>
            <milo-context-consumer-wrapper-test></milo-context-consumer-wrapper-test>
          </milo-outer-context-provider-test>
        </div>
      `);
      const globalProvider = rootEle.querySelector('milo-global-context-provider-test')!.shadowRoot!
        .host as GlobalContextProvider;
      const consumer = rootEle
        .querySelector('milo-context-consumer-wrapper-test')!
        .shadowRoot!.querySelector('milo-context-consumer-test')!.shadowRoot!.host as ContextConsumer;

      assert.strictEqual(consumer.providerKey, 'outer_provider-provider-val0');

      // Update global provider.
      globalProvider.providerKey = 'global_provider-provider-val1';
      await globalProvider.updateComplete;

      // consumer.providerKey not updated.
      assert.strictEqual(consumer.providerKey, 'outer_provider-provider-val0');
    });
  });
});
