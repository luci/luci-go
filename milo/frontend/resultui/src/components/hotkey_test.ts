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

describe('hotkey_test', () => {
  let hotkeyEle: HotkeyElement;
  let childEle: Element;
  const handlerSpy = sinon.spy();
  let handlerSpyCallCount = 0;
  const handlerSpy2 = sinon.spy();
  let handlerSpy2CallCount = 0;

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
    childEle.dispatchEvent(new KeyboardEvent('keydown', {bubbles: true, composed: true, keyCode: 'A'.charCodeAt(0)} as KeyboardEventInit));
    childEle.dispatchEvent(new KeyboardEvent('keyup', {bubbles: true, composed: true, keyCode: 'A'.charCodeAt(0)} as KeyboardEventInit));
    assert.equal(handlerSpy.callCount, ++handlerSpyCallCount);
  });

  it('should react to key press outside of the child element', () => {
    document.dispatchEvent(new KeyboardEvent('keydown', {bubbles: true, composed: true, keyCode: 'A'.charCodeAt(0)} as KeyboardEventInit));
    childEle.dispatchEvent(new KeyboardEvent('keyup', {bubbles: true, composed: true, keyCode: 'A'.charCodeAt(0)} as KeyboardEventInit));
    assert.equal(handlerSpy.callCount, ++handlerSpyCallCount);
  });

  it('should react to the new key press event when the key is updated', async () => {
    hotkeyEle.key = 'b';
    await hotkeyEle.updateComplete;

    document.dispatchEvent(new KeyboardEvent('keydown', {bubbles: true, composed: true, keyCode: 'A'.charCodeAt(0)} as KeyboardEventInit));
    childEle.dispatchEvent(new KeyboardEvent('keyup', {bubbles: true, composed: true, keyCode: 'A'.charCodeAt(0)} as KeyboardEventInit));
    assert.equal(handlerSpy.callCount, handlerSpyCallCount);

    document.dispatchEvent(new KeyboardEvent('keydown', {bubbles: true, composed: true, keyCode: 'B'.charCodeAt(0)} as KeyboardEventInit));
    childEle.dispatchEvent(new KeyboardEvent('keyup', {bubbles: true, composed: true, keyCode: 'B'.charCodeAt(0)} as KeyboardEventInit));
    assert.equal(handlerSpy.callCount, ++handlerSpyCallCount);
  });

  it('should trigger the new handler when the handler is updated', async () => {
    hotkeyEle.handler = handlerSpy2;
    await hotkeyEle.updateComplete;

    document.dispatchEvent(new KeyboardEvent('keydown', {bubbles: true, composed: true, keyCode: 'B'.charCodeAt(0)} as KeyboardEventInit));
    childEle.dispatchEvent(new KeyboardEvent('keyup', {bubbles: true, composed: true, keyCode: 'B'.charCodeAt(0)} as KeyboardEventInit));
    assert.equal(handlerSpy.callCount, handlerSpyCallCount);
    assert.equal(handlerSpy2.callCount, ++handlerSpy2CallCount);
  });

  it('should not trigger the handler when the component is disconnected', () => {
    hotkeyEle.disconnectedCallback();

    document.dispatchEvent(new KeyboardEvent('keydown', {bubbles: true, composed: true, keyCode: 'B'.charCodeAt(0)} as KeyboardEventInit));
    childEle.dispatchEvent(new KeyboardEvent('keyup', {bubbles: true, composed: true, keyCode: 'B'.charCodeAt(0)} as KeyboardEventInit));
    assert.equal(handlerSpy2.callCount, handlerSpy2CallCount);
  });
});
