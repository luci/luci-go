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

import { fixture } from '@open-wc/testing';
import { assert } from 'chai';
import { customElement, html, LitElement, property } from 'lit-element';

import { buildContextProviderMixin, buildContextSubscriberMixin } from './context';

interface Context {
  // Provided by outer provider.
  // But not associated with a @property of the provider.
  outerProviderDefaultKey: string;

  // Provided by outer provider.
  // But only associated with a unannotated property of the provider.
  outerProviderInactiveKey: string;

  // Provided by the outer provider.
  outerProviderKey: string;

  // Provided by both the outer provider and the inner provider.
  providerKey: string;

  // Unprovided and but observed key.
  unprovidedKey: string;

  // Provided but unobserved key.
  unobservedKey: string;
}

const DEFAULT_OUTER_CONTEXT = {
  outerProviderDefaultKey: 'outer_provider-outer_provider_default-default',
  outerProviderInactiveKey: 'outer_provider-outer_provider_inactive-default',
  outerProviderKey: 'outer_provider-outer_provider-default',
  providerKey: 'outer_provider-provider-default',
  unobservedKey: 'outer_provider-unobserved-default',
};
@customElement('tr-outer-context-provider-test')
class OuterContextProvider extends buildContextProviderMixin(DEFAULT_OUTER_CONTEXT)(LitElement) {
  outerProviderInactiveKey = 'outer_provider-outer_provider_inactive-val0';

  @property()
  outerProviderKey = 'outer_provider-outer_provider-val0';

  @property()
  providerKey = 'outer_provider-provider-val0';

  @property()
  unobservedKey = 'outer_provider-unobserved-val0';

  @property()
  localKey = 'outer_provider-local-val0';
}

const DEFAULT_INNER_CONTEXT = {
  providerKey: 'inner_provider-provider-default',
};
@customElement('tr-inner-context-provider-test')
class InnerContextProvider extends buildContextProviderMixin(DEFAULT_INNER_CONTEXT)(LitElement) {
  @property()
  providerKey = 'inner_provider-provider-val0';
}

const observedKeys: Array<keyof Context> = [
  'outerProviderDefaultKey',
  'outerProviderInactiveKey',
  'outerProviderKey',
  'providerKey',
  'unprovidedKey',
];
@customElement('tr-context-subscriber-test')
class ContextSubscriber extends buildContextSubscriberMixin<Context>(observedKeys)(LitElement) {
  @property()
  outerProviderDefaultKey = 'local-outer_provider_default';

  @property()
  outerProviderKey = 'local-output_provider';

  @property()
  providerKey = 'local-provider';

  @property()
  unprovidedKey = 'local-unprovided';

  @property()
  unobservedKey = 'local-unobserved';
}

@customElement('tr-context-subscriber-wrapper-test')
export class ContextSubscriberWrapper extends LitElement {
  protected render() {
    return html`
      <tr-context-subscriber-test></tr-context-subscriber-test>
    `;
  }
}

describe('context', () => {
  describe('ContextProvider', () => {
    it('should provide context to descendent context subscribers', async () => {
      const outerProvider = await fixture<OuterContextProvider>(html`
        <tr-outer-context-provider-test>
          <tr-inner-context-provider-test>
            <tr-context-subscriber-test id="inner-subscriber">
            </tr-context-subscriber-test>
          </tr-inner-context-provider-test>
          <tr-context-subscriber-test id="outer-subscriber">
          </tr-context-subscriber-test>
        <tr-outer-context-provider>
      `);
      const innerProvider = outerProvider.querySelector('tr-inner-context-provider-test')!.shadowRoot!.host as InnerContextProvider;
      const outerSubscriber = outerProvider.querySelector('#outer-subscriber')!.shadowRoot!.host as ContextSubscriber;
      const innerSubscriber = outerProvider.querySelector('#inner-subscriber')!.shadowRoot!.host as ContextSubscriber;

      assert.strictEqual(outerSubscriber.outerProviderDefaultKey, 'outer_provider-outer_provider_default-default');
      assert.strictEqual(outerSubscriber.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(outerSubscriber.outerProviderKey, 'outer_provider-outer_provider-val0');
      assert.strictEqual(outerSubscriber.providerKey, 'outer_provider-provider-val0');
      assert.strictEqual(outerSubscriber.unprovidedKey, 'local-unprovided');
      assert.strictEqual(outerSubscriber.unobservedKey, 'local-unobserved');

      assert.strictEqual(innerSubscriber.outerProviderDefaultKey, 'outer_provider-outer_provider_default-default');
      assert.strictEqual(innerSubscriber.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(innerSubscriber.outerProviderKey, 'outer_provider-outer_provider-val0');
      assert.strictEqual(innerSubscriber.providerKey, 'inner_provider-provider-val0');
      assert.strictEqual(innerSubscriber.unprovidedKey, 'local-unprovided');
      assert.strictEqual(innerSubscriber.unobservedKey, 'local-unobserved');

      // Update outer provider.
      outerProvider.outerProviderDefaultKey = 'outer_provider-outer_provider_default-val1';
      outerProvider.outerProviderInactiveKey = 'outer_provider-outer_provider_inactive-val1';
      outerProvider.outerProviderKey = 'outer_provider-outer_provider-val1';
      outerProvider.providerKey = 'outer_provider-provider-val1';
      outerProvider.unobservedKey = 'outer_provider-unobserved_val1';
      outerProvider.localKey = 'outer_provider-local_val1';
      await outerProvider.updateComplete;

      // outerSubscriber updated.
      assert.strictEqual(outerSubscriber.outerProviderDefaultKey, 'outer_provider-outer_provider_default-default');
      assert.strictEqual(outerSubscriber.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(outerSubscriber.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(outerSubscriber.providerKey, 'outer_provider-provider-val1');
      assert.strictEqual(outerSubscriber.unprovidedKey, 'local-unprovided');
      assert.strictEqual(outerSubscriber.unobservedKey, 'local-unobserved');

      // innerSubscriber.providerKey unchanged, other properties updated.
      assert.strictEqual(innerSubscriber.outerProviderDefaultKey, 'outer_provider-outer_provider_default-default');
      assert.strictEqual(innerSubscriber.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(innerSubscriber.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(innerSubscriber.providerKey, 'inner_provider-provider-val0');
      assert.strictEqual(innerSubscriber.unprovidedKey, 'local-unprovided');
      assert.strictEqual(innerSubscriber.unobservedKey, 'local-unobserved');

      // Update inner provider.
      innerProvider.providerKey = 'inner_provider-provider-val1';
      await innerProvider.updateComplete;

      // outerSubscriber unchanged.
      assert.strictEqual(outerSubscriber.outerProviderDefaultKey, 'outer_provider-outer_provider_default-default');
      assert.strictEqual(outerSubscriber.outerProviderInactiveKey, 'outer_provider-outer_provider_inactive-val0');
      assert.strictEqual(outerSubscriber.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(outerSubscriber.providerKey, 'outer_provider-provider-val1');
      assert.strictEqual(outerSubscriber.unprovidedKey, 'local-unprovided');
      assert.strictEqual(outerSubscriber.unobservedKey, 'local-unobserved');

      // innerSubscriber.providerKey updated, other properties unchanged.
      assert.strictEqual(innerSubscriber.outerProviderDefaultKey, 'outer_provider-outer_provider_default-default');
      assert.strictEqual(innerSubscriber.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(innerSubscriber.providerKey, 'inner_provider-provider-val1');
      assert.strictEqual(innerSubscriber.unprovidedKey, 'local-unprovided');
      assert.strictEqual(innerSubscriber.unobservedKey, 'local-unobserved');
    });

    it('should provide context to context subscribers in shadow DOMs', async () => {
      const outerProvider = await fixture<OuterContextProvider>(html`
        <tr-outer-context-provider-test>
          <tr-inner-context-provider-test>
            <tr-context-subscriber-wrapper-test>
            </tr-context-subscriber-wrapper-test>
          </tr-inner-context-provider-test>
        </tr-outer-context-provider-test>
      `);
      const innerProvider = outerProvider.querySelector('tr-inner-context-provider-test')!.shadowRoot!.host as InnerContextProvider;
      const subscriber = innerProvider.querySelector('tr-context-subscriber-wrapper-test')!.shadowRoot!
        .querySelector('tr-context-subscriber-test')!.shadowRoot!.host as ContextSubscriber;

      assert.strictEqual(subscriber.outerProviderDefaultKey, 'outer_provider-outer_provider_default-default');
      assert.strictEqual(subscriber.outerProviderKey, 'outer_provider-outer_provider-val0');
      assert.strictEqual(subscriber.providerKey, 'inner_provider-provider-val0');
      assert.strictEqual(subscriber.unprovidedKey, 'local-unprovided');
      assert.strictEqual(subscriber.unobservedKey, 'local-unobserved');

      // Update outer provider.
      outerProvider.outerProviderDefaultKey = 'outer_provider-outer_provider_default-val1';
      outerProvider.outerProviderKey = 'outer_provider-outer_provider-val1';
      outerProvider.providerKey = 'outer_provider-provider-val1';
      outerProvider.unobservedKey = 'outer_provider-unobserved_val1';
      outerProvider.localKey = 'outer_provider-local_val1';
      await outerProvider.updateComplete;

      // subscriber.providerKey unchanged, other properties updated.
      assert.strictEqual(subscriber.outerProviderDefaultKey, 'outer_provider-outer_provider_default-default');
      assert.strictEqual(subscriber.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(subscriber.providerKey, 'inner_provider-provider-val0');
      assert.strictEqual(subscriber.unprovidedKey, 'local-unprovided');
      assert.strictEqual(subscriber.unobservedKey, 'local-unobserved');

      // Update inner provider.
      innerProvider.providerKey = 'inner_provider-provider-val1';
      await innerProvider.updateComplete;

      // subscriber.providerKey updated, other properties unchanged.
      assert.strictEqual(subscriber.outerProviderDefaultKey, 'outer_provider-outer_provider_default-default');
      assert.strictEqual(subscriber.outerProviderKey, 'outer_provider-outer_provider-val1');
      assert.strictEqual(subscriber.providerKey, 'inner_provider-provider-val1');
      assert.strictEqual(subscriber.unprovidedKey, 'local-unprovided');
      assert.strictEqual(subscriber.unobservedKey, 'local-unobserved');
    });
  });
});
