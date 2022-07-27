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

import { fixture, fixtureCleanup, html } from '@open-wc/testing/index-no-side-effects';
import { assert } from 'chai';
import { customElement, LitElement } from 'lit-element';
import sinon from 'sinon';

import './hotkey';
import { HotkeyElement } from './hotkey';

function simulateKeyStroke(target: EventTarget, key: string) {
  target.dispatchEvent(
    new KeyboardEvent('keydown', {
      bubbles: true,
      composed: true,
      keyCode: key.toUpperCase().charCodeAt(0),
    } as KeyboardEventInit)
  );
  target.dispatchEvent(
    new KeyboardEvent('keyup', {
      bubbles: true,
      composed: true,
      keyCode: key.toUpperCase().charCodeAt(0),
    } as KeyboardEventInit)
  );
}

@customElement('milo-hotkey-test-wrapper')
class WrapperElement extends LitElement {
  protected render() {
    return html`
      <input id="input">
      <select id="select"></select>
      <textarea id="textarea">
    `;
  }
}

describe('hotkey_test', () => {
  let hotkeyEle: HotkeyElement;
  let childEle: Element;
  let inputEle: HTMLInputElement;
  let selectEle: HTMLSelectElement;
  let textareaEle: HTMLTextAreaElement;
  let wrappedInputEle: HTMLInputElement;
  let wrappedSelectEle: HTMLSelectElement;
  let wrappedTextareaEle: HTMLTextAreaElement;
  const handlerSpy = sinon.spy();
  const handlerSpy2 = sinon.spy();

  before(async () => {
    const parentEle = await fixture<HotkeyElement>(html`
      <div>
        <milo-hotkey id="hotkey" .key=${'a'} .handler=${handlerSpy}>
          <div id="child"></div>
        </milo-hotkey>
        <input id="input" />
        <select id="select"></select>
        <textarea id="textarea"></textarea>
        <milo-hotkey-test-wrapper id="wrapped"></milo-hotkey-test-wrapper>
      </div>
    `);
    hotkeyEle = parentEle.querySelector<HotkeyElement>('#hotkey')!;
    childEle = parentEle.querySelector('#child')!;
    inputEle = parentEle.querySelector<HTMLInputElement>('#input')!;
    selectEle = parentEle.querySelector<HTMLSelectElement>('#select')!;
    textareaEle = parentEle.querySelector<HTMLTextAreaElement>('#textarea')!;

    const wrapped = parentEle.querySelector<WrapperElement>('#wrapped')!;
    wrappedInputEle = wrapped.shadowRoot!.querySelector<HTMLInputElement>('#input')!;
    wrappedSelectEle = wrapped.shadowRoot!.querySelector<HTMLSelectElement>('#select')!;
    wrappedTextareaEle = wrapped.shadowRoot!.querySelector<HTMLTextAreaElement>('#textarea')!;
  });
  after(fixtureCleanup);

  it('should react to key press on the child element', () => {
    simulateKeyStroke(childEle, 'a');
    assert.equal(handlerSpy.callCount, 1);
  });

  it('should react to key press outside of the child element', () => {
    simulateKeyStroke(document, 'a');
    assert.equal(handlerSpy.callCount, 2);
  });

  it('should react to the new key press event when the key is updated', async () => {
    hotkeyEle.key = 'b';
    await hotkeyEle.updateComplete;

    simulateKeyStroke(document, 'a');
    assert.equal(handlerSpy.callCount, 2);

    simulateKeyStroke(document, 'b');
    assert.equal(handlerSpy.callCount, 3);
  });

  it('should trigger the new handler when the handler is updated', async () => {
    hotkeyEle.handler = handlerSpy2;
    await hotkeyEle.updateComplete;

    simulateKeyStroke(document, 'b');
    assert.equal(handlerSpy.callCount, 3);
    assert.equal(handlerSpy2.callCount, 1);
  });

  it('should not trigger the handler when the target element is INPUT/SELECT/TEXTAREA', () => {
    simulateKeyStroke(inputEle, 'b');
    simulateKeyStroke(selectEle, 'b');
    simulateKeyStroke(textareaEle, 'b');
    assert.equal(handlerSpy2.callCount, 1);
  });

  it('should not trigger the handler when the target element is INPUT/SELECT/TEXTAREA in a web component', () => {
    simulateKeyStroke(wrappedInputEle, 'b');
    simulateKeyStroke(wrappedSelectEle, 'b');
    simulateKeyStroke(wrappedTextareaEle, 'b');
    assert.equal(handlerSpy2.callCount, 1);
  });

  it('should not trigger the handler when the component is disconnected', () => {
    hotkeyEle.disconnectedCallback();

    simulateKeyStroke(document, 'b');
    assert.equal(handlerSpy2.callCount, 1);
  });
});
