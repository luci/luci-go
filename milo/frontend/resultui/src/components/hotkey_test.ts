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
import sinon from 'sinon';

import './hotkey';
import { HotkeyElement } from './hotkey';

function simulateKeyStroke(target: EventTarget, key: string) {
  target.dispatchEvent(new KeyboardEvent('keydown', {bubbles: true, composed: true, keyCode: key.toUpperCase().charCodeAt(0)} as KeyboardEventInit));
  target.dispatchEvent(new KeyboardEvent('keyup', {bubbles: true, composed: true, keyCode: key.toUpperCase().charCodeAt(0)} as KeyboardEventInit));
}

describe('hotkey_test', () => {
  let hotkeyEle: HotkeyElement;
  let childEle: Element;
  const handlerSpy = sinon.spy();
  const handlerSpy2 = sinon.spy();

  before(async() => {
    hotkeyEle = await fixture<HotkeyElement>(html`
      <milo-hotkey
        key="a"
        .handler=${handlerSpy}
      >
        <div id="child"></div>
      </milo-hotkey>
    `);
    childEle = hotkeyEle.querySelector('#child')!;
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

  it('should not trigger the handler when the component is disconnected', () => {
    hotkeyEle.disconnectedCallback();

    simulateKeyStroke(document, 'b');
    assert.equal(handlerSpy2.callCount, 1);
  });
});
