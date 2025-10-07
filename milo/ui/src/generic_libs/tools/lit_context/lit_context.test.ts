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

import { fixture } from '@open-wc/testing-helpers';
import { html, LitElement } from 'lit';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable, reaction } from 'mobx';

import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';

import { consumer, createContextLink, provider } from './lit_context';

class Parent {
  prop1 = 1;
}

class Child extends Parent {
  prop2 = 2;
}

class GrandChild extends Child {
  prop3 = 3;
}

const [provideOuterProviderInactiveKey, consumeOuterProviderInactiveKey] =
  createContextLink<string>();
const [provideOuterProviderKey, consumeOuterProviderKey] =
  createContextLink<string>();
const [provideProviderKey, consumeProviderKey] = createContextLink<string>();
const [, consumeUnprovidedKey] = createContextLink<string>();
const [provideOuterUnobservedKey] = createContextLink<string>();
const [provideConsumerOptionalPropKey, consumeConsumerOptionalPropKey] =
  createContextLink<string>();
const [provideSubtypeKey, consumeSubtypeKey] = createContextLink<Child>();
const [provideSelfProvidedKey, consumeSelfProvidedKey] =
  createContextLink<string>();

@customElement('milo-outer-context-provider-test')
@provider
class OuterContextProvider extends MobxExtLitElement {
  @provideOuterProviderInactiveKey()
  outerProviderInactiveKey = 'outer_provider-outer_provider_inactive-val0';

  @observable.ref
  @provideOuterProviderKey()
  outerProviderKey = 'outer_provider-outer_provider-val0';

  // The same context is provided by multiple properties, the last one is used.
  @observable.ref
  @provideProviderKey()
  ignoredProviderKey = 'ignored_outer_provider-provider-val0';

  @observable.ref
  @provideProviderKey()
  providerKey = 'outer_provider-provider-val0';

  @observable.ref
  @provideOuterUnobservedKey()
  outerUnobservedKey = 'outer_provider-outer_unobserved-val0';

  @observable.ref
  unprovidedKey = 'outer_provider-unprovided-val0';

  @observable.ref
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

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback(): void {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.outerProviderKey,
        (outerProviderKey) => {
          // Emulate @property() update.
          this.updated(new Map([['outerProviderKey', outerProviderKey]]));
        },
        { fireImmediately: true },
      ),
    );
    this.addDisposer(
      reaction(
        () => this.ignoredProviderKey,
        (ignoredProviderKey) => {
          // Emulate @property() update.
          this.updated(new Map([['ignoredProviderKey', ignoredProviderKey]]));
        },
        { fireImmediately: true },
      ),
    );
    this.addDisposer(
      reaction(
        () => this.providerKey,
        (providerKey) => {
          // Emulate @property() update.
          this.updated(new Map([['providerKey', providerKey]]));
        },
        { fireImmediately: true },
      ),
    );
    this.addDisposer(
      reaction(
        () => this.outerUnobservedKey,
        (outerUnobservedKey) => {
          // Emulate @property() update.
          this.updated(new Map([['outerUnobservedKey', outerUnobservedKey]]));
        },
        { fireImmediately: true },
      ),
    );
    this.addDisposer(
      reaction(
        () => this.unprovidedKey,
        (unprovidedKey) => {
          // Emulate @property() update.
          this.updated(new Map([['unprovidedKey', unprovidedKey]]));
        },
        { fireImmediately: true },
      ),
    );
    this.addDisposer(
      reaction(
        () => this.consumerOptionalPropKey,
        (consumerOptionalPropKey) => {
          // Emulate @property() update.
          this.updated(
            new Map([['consumerOptionalPropKey', consumerOptionalPropKey]]),
          );
        },
        { fireImmediately: true },
      ),
    );
  }
}

@customElement('milo-inner-context-provider-test')
@provider
class InnerContextProvider extends MobxExtLitElement {
  @observable.ref
  @provideProviderKey()
  providerKey = 'inner_provider-provider-val0';

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback(): void {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.providerKey,
        (providerKey) => {
          // Emulate @property() update.
          this.updated(new Map([['providerKey', providerKey]]));
        },
        { fireImmediately: true },
      ),
    );
  }
}

@customElement('milo-global-context-provider-test')
@provider
class GlobalContextProvider extends MobxExtLitElement {
  @observable.ref
  @provideProviderKey({ global: true })
  providerKey = 'global_provider-provider-val0';

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback(): void {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.providerKey,
        (providerKey) => {
          // Emulate @property() update.
          this.updated(new Map([['providerKey', providerKey]]));
        },
        { fireImmediately: true },
      ),
    );
  }
}

@customElement('milo-context-consumer-test')
@provider
@consumer
class ContextConsumer extends MobxExtLitElement {
  @consumeOuterProviderInactiveKey()
  outerProviderInactiveKey = 'local-outer_provider_inactive';

  @observable.ref
  @consumeOuterProviderKey()
  outerProviderKey = 'local-output_provider';

  @consumeProviderKey()
  providerKey = 'local-provider';

  // Multiple props can map to the same context.
  @consumeProviderKey()
  providerKeyWithAnotherName = 'local-provider';

  @observable.ref
  @consumeUnprovidedKey()
  unprovidedKey = 'local-unprovided';

  @observable.ref
  outerUnobservedKey = 'local-unobserved';

  // This should compile without warnings.
  @observable.ref
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

  @observable.ref
  @provideSelfProvidedKey()
  selfProvided = 'local-self_provided';

  @observable.ref
  @consumeSelfProvidedKey()
  selfProvided2 = '';

  // Help testing that the property is not set too many times, particularly
  // when the element is first rendered.
  setterCallCount = 0;
  @consumeProviderKey()
  set providerKeySetter(_newVal: string) {
    this.setterCallCount++;
  }

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback(): void {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.selfProvided,
        (selfProvided) => {
          // Emulate @property() update.
          this.updated(new Map([['selfProvided', selfProvided]]));
        },
        { fireImmediately: true },
      ),
    );
    this.addDisposer(
      reaction(
        () => this.selfProvided2,
        (selfProvided2) => {
          // Emulate @property() update.
          this.updated(new Map([['selfProvided2', selfProvided2]]));
        },
        { fireImmediately: true },
      ),
    );
  }
}

@customElement('milo-context-consumer-wrapper-test')
class ContextConsumerWrapper extends LitElement {
  protected render() {
    return html` <milo-context-consumer-test></milo-context-consumer-test> `;
  }
}
void ContextConsumerWrapper;

// TODO(weiweilin): test what happens when ContextProvider is disconnected from
// DOM then reconnected to DOM.

describe('context', () => {
  test('should provide context to descendent context consumers', async () => {
    const outerProvider = await fixture<OuterContextProvider>(html`
      <milo-outer-context-provider-test>
        <milo-inner-context-provider-test>
          <milo-context-consumer-test id="inner-consumer">
          </milo-context-consumer-test>
        </milo-inner-context-provider-test>
        <milo-context-consumer-test id="outer-consumer">
        </milo-context-consumer-test>
        <milo-outer-context-provider> </milo-outer-context-provider
      ></milo-outer-context-provider-test>
    `);

    const innerProvider = outerProvider.querySelector(
      'milo-inner-context-provider-test',
    )!.shadowRoot!.host as InnerContextProvider;
    const outerConsumer = outerProvider.querySelector('#outer-consumer')!
      .shadowRoot!.host as ContextConsumer;
    const innerConsumer = outerProvider.querySelector('#inner-consumer')!
      .shadowRoot!.host as ContextConsumer;

    expect(outerConsumer.outerProviderInactiveKey).toStrictEqual(
      'outer_provider-outer_provider_inactive-val0',
    );
    expect(outerConsumer.outerProviderKey).toStrictEqual(
      'outer_provider-outer_provider-val0',
    );
    expect(outerConsumer.providerKey).toStrictEqual(
      'outer_provider-provider-val0',
    );
    expect(outerConsumer.providerKeyWithAnotherName).toStrictEqual(
      'outer_provider-provider-val0',
    );
    expect(outerConsumer.outerUnobservedKey).toStrictEqual('local-unobserved');
    expect(outerConsumer.unprovidedKey).toStrictEqual('local-unprovided');
    expect(outerConsumer.selfProvided2).toStrictEqual('local-self_provided');
    expect(outerConsumer.setterCallCount).toStrictEqual(1);

    expect(innerConsumer.outerProviderInactiveKey).toStrictEqual(
      'outer_provider-outer_provider_inactive-val0',
    );
    expect(innerConsumer.outerProviderKey).toStrictEqual(
      'outer_provider-outer_provider-val0',
    );
    expect(innerConsumer.providerKey).toStrictEqual(
      'inner_provider-provider-val0',
    );
    expect(innerConsumer.providerKeyWithAnotherName).toStrictEqual(
      'inner_provider-provider-val0',
    );
    expect(innerConsumer.outerUnobservedKey).toStrictEqual('local-unobserved');
    expect(innerConsumer.unprovidedKey).toStrictEqual('local-unprovided');
    expect(innerConsumer.setterCallCount).toStrictEqual(1);

    // Update outer provider.
    outerProvider.outerProviderInactiveKey =
      'outer_provider-outer_provider_inactive-val1';
    outerProvider.outerProviderKey = 'outer_provider-outer_provider-val1';
    outerProvider.providerKey = 'outer_provider-provider-val1';
    outerProvider.outerUnobservedKey = 'outer_provider-unobserved_val1';
    outerProvider.unprovidedKey = 'outer_provider-local_val1';
    await outerProvider.updateComplete;

    // outerConsumer updated.
    expect(outerConsumer.outerProviderInactiveKey).toStrictEqual(
      'outer_provider-outer_provider_inactive-val0',
    );
    expect(outerConsumer.outerProviderKey).toStrictEqual(
      'outer_provider-outer_provider-val1',
    );
    expect(outerConsumer.providerKey).toStrictEqual(
      'outer_provider-provider-val1',
    );
    expect(outerConsumer.providerKeyWithAnotherName).toStrictEqual(
      'outer_provider-provider-val1',
    );
    expect(outerConsumer.outerUnobservedKey).toStrictEqual('local-unobserved');
    expect(outerConsumer.unprovidedKey).toStrictEqual('local-unprovided');
    expect(outerConsumer.setterCallCount).toStrictEqual(2);

    // innerConsumer.providerKey unchanged, other properties updated.
    expect(innerConsumer.outerProviderInactiveKey).toStrictEqual(
      'outer_provider-outer_provider_inactive-val0',
    );
    expect(innerConsumer.outerProviderKey).toStrictEqual(
      'outer_provider-outer_provider-val1',
    );
    expect(innerConsumer.providerKey).toStrictEqual(
      'inner_provider-provider-val0',
    );
    expect(innerConsumer.providerKeyWithAnotherName).toStrictEqual(
      'inner_provider-provider-val0',
    );
    expect(innerConsumer.unprovidedKey).toStrictEqual('local-unprovided');
    expect(innerConsumer.outerUnobservedKey).toStrictEqual('local-unobserved');
    expect(innerConsumer.setterCallCount).toStrictEqual(1);

    // Update inner provider.
    innerProvider.providerKey = 'inner_provider-provider-val1';
    await innerProvider.updateComplete;

    // outerConsumer unchanged.
    expect(outerConsumer.outerProviderInactiveKey).toStrictEqual(
      'outer_provider-outer_provider_inactive-val0',
    );
    expect(outerConsumer.outerProviderKey).toStrictEqual(
      'outer_provider-outer_provider-val1',
    );
    expect(outerConsumer.providerKey).toStrictEqual(
      'outer_provider-provider-val1',
    );
    expect(outerConsumer.providerKeyWithAnotherName).toStrictEqual(
      'outer_provider-provider-val1',
    );
    expect(outerConsumer.outerUnobservedKey).toStrictEqual('local-unobserved');
    expect(outerConsumer.unprovidedKey).toStrictEqual('local-unprovided');
    expect(outerConsumer.setterCallCount).toStrictEqual(2);

    // innerConsumer.providerKey updated, other properties unchanged.
    expect(innerConsumer.outerProviderInactiveKey).toStrictEqual(
      'outer_provider-outer_provider_inactive-val0',
    );
    expect(innerConsumer.outerProviderKey).toStrictEqual(
      'outer_provider-outer_provider-val1',
    );
    expect(innerConsumer.providerKey).toStrictEqual(
      'inner_provider-provider-val1',
    );
    expect(innerConsumer.providerKeyWithAnotherName).toStrictEqual(
      'inner_provider-provider-val1',
    );
    expect(innerConsumer.outerUnobservedKey).toStrictEqual('local-unobserved');
    expect(innerConsumer.unprovidedKey).toStrictEqual('local-unprovided');
    expect(innerConsumer.setterCallCount).toStrictEqual(2);
  });

  test('should provide context to any context consumers if global is set to true', async () => {
    const rootEle = await fixture<HTMLDivElement>(html`
      <div>
        <milo-outer-context-provider-test></milo-outer-context-provider-test>
        <milo-global-context-provider-test></milo-global-context-provider-test>
        <milo-context-consumer-wrapper-test></milo-context-consumer-wrapper-test>
      </div>
    `);
    const globalProvider = rootEle.querySelector(
      'milo-global-context-provider-test',
    )!.shadowRoot!.host as GlobalContextProvider;
    const consumer = rootEle
      .querySelector('milo-context-consumer-wrapper-test')!
      .shadowRoot!.querySelector('milo-context-consumer-test')!.shadowRoot!
      .host as ContextConsumer;

    expect(consumer.providerKey).toStrictEqual('global_provider-provider-val0');

    // Update global provider.
    globalProvider.providerKey = 'global_provider-provider-val1';
    await globalProvider.updateComplete;

    // consumer.providerKey updated
    expect(consumer.providerKey).toStrictEqual('global_provider-provider-val1');
  });

  test('global context should not override local context', async () => {
    const rootEle = await fixture<HTMLDivElement>(html`
      <div>
        <milo-global-context-provider-test></milo-global-context-provider-test>
        <milo-outer-context-provider-test>
          <milo-context-consumer-wrapper-test></milo-context-consumer-wrapper-test>
        </milo-outer-context-provider-test>
      </div>
    `);
    const globalProvider = rootEle.querySelector(
      'milo-global-context-provider-test',
    )!.shadowRoot!.host as GlobalContextProvider;
    const consumer = rootEle
      .querySelector('milo-context-consumer-wrapper-test')!
      .shadowRoot!.querySelector('milo-context-consumer-test')!.shadowRoot!
      .host as ContextConsumer;

    expect(consumer.providerKey).toStrictEqual('outer_provider-provider-val0');

    // Update global provider.
    globalProvider.providerKey = 'global_provider-provider-val1';
    await globalProvider.updateComplete;

    // consumer.providerKey not updated.
    expect(consumer.providerKey).toStrictEqual('outer_provider-provider-val0');
  });
});
