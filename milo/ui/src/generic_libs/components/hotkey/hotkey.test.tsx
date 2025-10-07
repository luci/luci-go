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

import { render, screen } from '@testing-library/react';
import { HotkeysEvent } from 'hotkeys-js';
import { html, LitElement } from 'lit';
import { customElement } from 'lit/decorators.js';

import './hotkey';
import { Hotkey } from './hotkey';

function simulateKeyStroke(target: EventTarget, key: string) {
  target.dispatchEvent(
    new KeyboardEvent('keydown', {
      bubbles: true,
      composed: true,
      keyCode: key.toUpperCase().charCodeAt(0),
    } as KeyboardEventInit),
  );
  target.dispatchEvent(
    new KeyboardEvent('keyup', {
      bubbles: true,
      composed: true,
      keyCode: key.toUpperCase().charCodeAt(0),
    } as KeyboardEventInit),
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
void WrapperElement; // 'Use' the class to silence eslint/tsc warnings.

declare module 'react' {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      ['milo-hotkey-test-wrapper']: { ['data-testid']: string };
    }
  }
}

describe('Hotkey', () => {
  let handlerSpy: jest.MockedFunction<
    (keyboardEvent: KeyboardEvent, hotkeysEvent: HotkeysEvent) => void
  >;
  let handlerSpy2: jest.MockedFunction<
    (keyboardEvent: KeyboardEvent, hotkeysEvent: HotkeysEvent) => void
  >;
  let ele: ReturnType<typeof render>;
  let childEle: Element;
  let inputEle: HTMLInputElement;
  let selectEle: HTMLSelectElement;
  let textareaEle: HTMLTextAreaElement;
  let wrappedInputEle: HTMLInputElement;
  let wrappedSelectEle: HTMLSelectElement;
  let wrappedTextareaEle: HTMLTextAreaElement;

  beforeEach(async () => {
    jest.useFakeTimers();
    handlerSpy = jest.fn(
      (_keyboardEvent: KeyboardEvent, _hotkeysEvent: HotkeysEvent) => {},
    );
    handlerSpy2 = jest.fn(
      (_keyboardEvent: KeyboardEvent, _hotkeysEvent: HotkeysEvent) => {},
    );

    ele = render(
      <Hotkey hotkey="a" handler={handlerSpy}>
        <div data-testid="child"></div>
        <input data-testid="input" />
        <select data-testid="select"></select>
        <textarea data-testid="textarea"></textarea>
        <milo-hotkey-test-wrapper data-testid="wrapped"></milo-hotkey-test-wrapper>
      </Hotkey>,
    );
    await jest.runAllTimersAsync();

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
    jest.useRealTimers();
  });

  test('should react to key press on the child element', () => {
    simulateKeyStroke(childEle, 'a');
    expect(handlerSpy.mock.calls.length).toStrictEqual(1);
  });

  test('should react to key press outside of the child element', () => {
    simulateKeyStroke(document, 'a');
    expect(handlerSpy.mock.calls.length).toStrictEqual(1);
  });

  test('should react to the new key press event when the key is updated', async () => {
    ele.rerender(
      <Hotkey hotkey="b" handler={handlerSpy}>
        <div data-testid="child"></div>
        <input data-testid="input" />
        <select data-testid="select"></select>
        <textarea data-testid="textarea"></textarea>
        <milo-hotkey-test-wrapper data-testid="wrapped"></milo-hotkey-test-wrapper>
      </Hotkey>,
    );
    await jest.runAllTimersAsync();

    simulateKeyStroke(document, 'a');
    expect(handlerSpy.mock.calls.length).toStrictEqual(0);

    simulateKeyStroke(document, 'b');
    expect(handlerSpy.mock.calls.length).toStrictEqual(1);
  });

  test('should trigger the new handler when the handler is updated', async () => {
    ele.rerender(
      <Hotkey hotkey="b" handler={handlerSpy2}>
        <div data-testid="child"></div>
        <input data-testid="input" />
        <select data-testid="select"></select>
        <textarea data-testid="textarea"></textarea>
        <milo-hotkey-test-wrapper data-testid="wrapped"></milo-hotkey-test-wrapper>
      </Hotkey>,
    );
    await jest.runAllTimersAsync();

    simulateKeyStroke(document, 'b');
    expect(handlerSpy.mock.calls.length).toStrictEqual(0);
    expect(handlerSpy2.mock.calls.length).toStrictEqual(1);
  });

  test('should not trigger the handler when the target element is INPUT/SELECT/TEXTAREA', () => {
    simulateKeyStroke(inputEle, 'b');
    simulateKeyStroke(selectEle, 'b');
    simulateKeyStroke(textareaEle, 'b');
    expect(handlerSpy2.mock.calls.length).toStrictEqual(0);
  });

  test('should not trigger the handler when the target element is INPUT/SELECT/TEXTAREA in a web component', () => {
    simulateKeyStroke(wrappedInputEle, 'b');
    simulateKeyStroke(wrappedSelectEle, 'b');
    simulateKeyStroke(wrappedTextareaEle, 'b');
    expect(handlerSpy2.mock.calls.length).toStrictEqual(0);
  });

  test('should not trigger the handler when the component is disconnected', async () => {
    ele.unmount();
    await jest.runAllTimersAsync();

    simulateKeyStroke(document, 'b');
    expect(handlerSpy2.mock.calls.length).toStrictEqual(0);
  });
});
