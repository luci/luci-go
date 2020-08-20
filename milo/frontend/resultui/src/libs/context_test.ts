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

import { fixture } from '@open-wc/testing';
import { assert } from 'chai';
import { customElement, html, LitElement, property } from 'lit-element';

import { consumeContext, provideContext } from './context';

@provideContext('outerProviderInactiveKey')
class OuterContextProvider extends LitElement {
  outerProviderInactiveKey = 'outer_provider-outer_provider_inactive-val0';

  @property()
  outerProviderKey = 'outer_provider-outer_provider-val0';

  @property()
  providerKey = 'outer_provider-provider-val0';

  @property()
  outerUnobservedKey = 'outer_provider-outer_unobserved-val0';

  @property()
  unprovidedKey = 'outer_provider-unprovided-val0';
}
customElement('milo-outer-context-provider-test')(
  provideContext('outerProviderInactiveKey')(
    provideContext('outerProviderKey')(
      provideContext('providerKey')(
        provideContext('outerUnobservedKey')(
          OuterContextProvider,
        ),
      ),
    ),
  ),
);


class InnerContextProvider extends LitElement {
  @property()
  providerKey = 'inner_provider-provider-val0';
}
customElement('milo-inner-context-provider-test')(
  provideContext('providerKey')(
    InnerContextProvider,
  ),
);


class ContextConsumer extends LitElement {
  @property()
  outerProviderInactiveKey = 'local-outer_provider_inactive';

  @property()
  outerProviderKey = 'local-output_provider';

  @property()
  providerKey = 'local-provider';

  @property()
  unprovidedKey = 'local-unprovided';

  @property()
  outerUnobservedKey = 'local-unobserved';
}
customElement('milo-context-consumer-test')(
  consumeContext('outerProviderInactiveKey')(
    consumeContext('outerProviderKey')(
      consumeContext('providerKey')(
        ContextConsumer,
      ),
    ),
  ),
);

@customElement('milo-context-consumer-wrapper-test')
export class ContextConsumerWrapper extends LitElement {
  protected render() {
    return html`
      <milo-context-consumer-test></milo-context-consumer-test>
    `;
  }
}


// TODO(weiweilin): test what happens when ContextProvider is disconnected from
// DOM then reconnected to DOM.
describe('context', () => {
  describe('ContextProvider', () => {
    it('should provide context to descendent context consumers', async () => {
      const outerProvider = await fixture<OuterContextProvider>(html`
        <milo-outer-context-provider-test>
          <milo-inner-context-provider-test>
            <milo-context-consumer-test id="inner-consumer">
            </milo-context-consumer-test>
          </milo-inner-context-provider-test>
          <milo-context-consumer-test id="outer-consumer">
          </milo-context-consumer-test>
        <milo-outer-context-provider>
      `);

      const innerProvider = outerProvider.querySelector('milo-inner-context-provider-test')!.shadowRoot!.host as InnerContextProvider;
      const outerConsumer = outerProvider.querySelector('#outer-consumer')!.shadowRoot!.host as ContextConsumer;
      const innerConsumer = outerProvider.querySelector('#inner-consumer')!.shadowRoot!.host as ContextConsumer;

      assert.strictEqual(outerConsumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(outerConsumer.outerProviderKey, 'outer_provider-outer_provider-val0');
      assert.strictEqual(outerConsumer.providerKey, 'outer_provider-provider-val0');
      assert.strictEqual(outerConsumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(outerConsumer.unprovidedKey, 'local-unprovided');

      assert.strictEqual(innerConsumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(innerConsumer.outerProviderKey, 'outer_provider-outer_provider-val0');
      assert.strictEqual(innerConsumer.providerKey, 'inner_provider-provider-val0');
      assert.strictEqual(innerConsumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(innerConsumer.unprovidedKey, 'local-unprovided');

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
      assert.strictEqual(outerConsumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(outerConsumer.unprovidedKey, 'local-unprovided');

      // innerConsumer.providerKey unchanged, other properties updated.
      assert.strictEqual(innerConsumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(innerConsumer.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(innerConsumer.providerKey, 'inner_provider-provider-val0');
      assert.strictEqual(innerConsumer.unprovidedKey, 'local-unprovided');
      assert.strictEqual(innerConsumer.outerUnobservedKey, 'local-unobserved');

      // Update inner provider.
      innerProvider.providerKey = 'inner_provider-provider-val1';
      await innerProvider.updateComplete;

      // outerConsumer unchanged.
      assert.strictEqual(outerConsumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(outerConsumer.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(outerConsumer.providerKey, 'outer_provider-provider-val1');
      assert.strictEqual(outerConsumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(outerConsumer.unprovidedKey, 'local-unprovided');

      // innerConsumer.providerKey updated, other properties unchanged.
      assert.strictEqual(innerConsumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(innerConsumer.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(innerConsumer.providerKey, 'inner_provider-provider-val1');
      assert.strictEqual(innerConsumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(innerConsumer.unprovidedKey, 'local-unprovided');
    });

    it('should provide context to context consumers in shadow DOMs', async () => {
      const outerProvider = await fixture<OuterContextProvider>(html`
        <milo-outer-context-provider-test>
          <milo-inner-context-provider-test>
            <milo-context-consumer-wrapper-test>
            </milo-context-consumer-wrapper-test>
          </milo-inner-context-provider-test>
        </milo-outer-context-provider-test>
      `);
      const innerProvider = outerProvider.querySelector('milo-inner-context-provider-test')!.shadowRoot!.host as InnerContextProvider;
      const consumer = innerProvider.querySelector('milo-context-consumer-wrapper-test')!.shadowRoot!
        .querySelector('milo-context-consumer-test')!.shadowRoot!.host as ContextConsumer;

      assert.strictEqual(consumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(consumer.outerProviderKey, 'outer_provider-outer_provider-val0');
      assert.strictEqual(consumer.providerKey, 'inner_provider-provider-val0');
      assert.strictEqual(consumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(consumer.unprovidedKey, 'local-unprovided');

      // Update outer provider.
      outerProvider.outerProviderInactiveKey = 'outer_provider-outer_provider_inactive-val1';
      outerProvider.outerProviderKey = 'outer_provider-outer_provider-val1';
      outerProvider.providerKey = 'outer_provider-provider-val1';
      outerProvider.outerUnobservedKey = 'outer_provider-unobserved_val1';
      outerProvider.unprovidedKey = 'outer_provider-unprovided_val1';
      await outerProvider.updateComplete;

      // consumer.providerKey unchanged, other properties updated.
      assert.strictEqual(consumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(consumer.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(consumer.providerKey, 'inner_provider-provider-val0');
      assert.strictEqual(consumer.unprovidedKey, 'local-unprovided');
      assert.strictEqual(consumer.outerUnobservedKey, 'local-unobserved');

      // Update inner provider.
      innerProvider.providerKey = 'inner_provider-provider-val1';
      await innerProvider.updateComplete;

      // consumer.providerKey updated, other properties unchanged.
      assert.strictEqual(consumer.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(consumer.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(consumer.providerKey, 'inner_provider-provider-val1');
      assert.strictEqual(consumer.outerUnobservedKey, 'local-unobserved');
      assert.strictEqual(consumer.unprovidedKey, 'local-unprovided');
    });
  });
});
