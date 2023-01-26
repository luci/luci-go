// Copyright 2023 The LUCI Authors.
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

import { render, RenderResult, screen } from '@testing-library/react';
import { expect } from 'chai';
import { customElement, html, LitElement } from 'lit-element';
import sinon from 'sinon';

import './hotkey';
import { Hotkey } from './hotkey';

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
export class WrapperElement extends LitElement {
  protected render() {
    return html`
      <input id="input">
      <select id="select"></select>
      <textarea id="textarea">
    `;
  }
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      ['milo-hotkey-test-wrapper']: { ['data-testid']: string };
    }
  }
}

describe('hotkey_test', () => {
  let timer: sinon.SinonFakeTimers;
  let handlerSpy: ReturnType<typeof sinon.spy>;
  let handlerSpy2: ReturnType<typeof sinon.spy>;

  let ele: RenderResult<typeof import('@testing-library/dom/types/queries'), HTMLElement, HTMLElement>;
  let childEle: Element;
  let inputEle: HTMLInputElement;
  let selectEle: HTMLSelectElement;
  let textareaEle: HTMLTextAreaElement;
  let wrappedInputEle: HTMLInputElement;
  let wrappedSelectEle: HTMLSelectElement;
  let wrappedTextareaEle: HTMLTextAreaElement;

  beforeEach(async () => {
    timer = sinon.useFakeTimers();
    handlerSpy = sinon.spy();
    handlerSpy2 = sinon.spy();

    ele = render(
      <Hotkey hotkey="a" handler={handlerSpy}>
        <div data-testid="child"></div>
        <input data-testid="input" />
        <select data-testid="select"></select>
        <textarea data-testid="textarea"></textarea>
        <milo-hotkey-test-wrapper data-testid="wrapped"></milo-hotkey-test-wrapper>
      </Hotkey>
    );
    await timer.runAllAsync();

    childEle = screen.getByTestId('child');
    inputEle = screen.getByTestId('input');
    selectEle = screen.getByTestId('select');
    textareaEle = screen.getByTestId('textarea');

    const wrapped = screen.getByTestId('wrapped');
    wrappedInputEle = wrapped.shadowRoot!.querySelector('#input')!;
    wrappedSelectEle = wrapped.shadowRoot!.querySelector('#select')!;
    wrappedTextareaEle = wrapped.shadowRoot!.querySelector('#textarea')!;
  });

  afterEach(() => {
    timer.restore();
  });

  it('should react to key press on the child element', () => {
    simulateKeyStroke(childEle, 'a');
    expect(handlerSpy.callCount).to.eq(1);
  });

  it('should react to key press outside of the child element', () => {
    simulateKeyStroke(document, 'a');
    expect(handlerSpy.callCount).to.eq(1);
  });

  it('should react to the new key press event when the key is updated', async () => {
    ele.rerender(
      <Hotkey hotkey="b" handler={handlerSpy}>
        <div data-testid="child"></div>
        <input data-testid="input" />
        <select data-testid="select"></select>
        <textarea data-testid="textarea"></textarea>
        <milo-hotkey-test-wrapper data-testid="wrapped"></milo-hotkey-test-wrapper>
      </Hotkey>
    );
    await timer.runAllAsync();

    simulateKeyStroke(document, 'a');
    expect(handlerSpy.callCount).to.eq(0);

    simulateKeyStroke(document, 'b');
    expect(handlerSpy.callCount).to.eq(1);
  });

  it('should trigger the new handler when the handler is updated', async () => {
    ele.rerender(
      <Hotkey hotkey="b" handler={handlerSpy2}>
        <div data-testid="child"></div>
        <input data-testid="input" />
        <select data-testid="select"></select>
        <textarea data-testid="textarea"></textarea>
        <milo-hotkey-test-wrapper data-testid="wrapped"></milo-hotkey-test-wrapper>
      </Hotkey>
    );
    await timer.runAllAsync();

    simulateKeyStroke(document, 'b');
    expect(handlerSpy.callCount).to.eq(0);
    expect(handlerSpy2.callCount).to.eq(1);
  });

  it('should not trigger the handler when the target element is INPUT/SELECT/TEXTAREA', () => {
    simulateKeyStroke(inputEle, 'b');
    simulateKeyStroke(selectEle, 'b');
    simulateKeyStroke(textareaEle, 'b');
    expect(handlerSpy2.callCount).to.eq(0);
  });

  it('should not trigger the handler when the target element is INPUT/SELECT/TEXTAREA in a web component', () => {
    simulateKeyStroke(wrappedInputEle, 'b');
    simulateKeyStroke(wrappedSelectEle, 'b');
    simulateKeyStroke(wrappedTextareaEle, 'b');
    expect(handlerSpy2.callCount).to.eq(0);
  });

  it('should not trigger the handler when the component is disconnected', async () => {
    ele.unmount();
    await timer.runAllAsync();

    simulateKeyStroke(document, 'b');
    expect(handlerSpy2.callCount).to.eq(0);
  });
});
