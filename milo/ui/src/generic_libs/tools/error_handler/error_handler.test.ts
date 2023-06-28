// Copyright 2021 The LUCI Authors.
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

import { MobxLitElement } from '@adobe/lit-mobx';
import { aTimeout, fixture, html } from '@open-wc/testing-helpers';
import { LitElement } from 'lit';
import { customElement } from 'lit/decorators.js';

import './error_handler';
import {
  errorHandler,
  forwardWithoutMsg,
  reportError,
  reportErrorAsync,
} from './error_handler';

@customElement('milo-error-handler-test-default')
@errorHandler()
class ErrorHandlerTestDefaultElement extends LitElement {
  protected render() {
    return html`<slot></slot>`;
  }
}

@customElement('milo-error-handler-test-on-error-returns-prop')
@errorHandler((err) => {
  err.stopPropagation();
  return false;
})
class ErrorHandlerTestOnErrorReturnsPropElement extends MobxLitElement {
  protected render() {
    return html`<slot></slot>`;
  }
}

describe('errorHandler', () => {
  test('should render error message by default', async () => {
    const errorHandlerEle = await fixture<ErrorHandlerTestDefaultElement>(html`
      <milo-error-handler-test-default>
        <div></div>
      </milo-error-handler-test-default>
    `);
    const childEle = errorHandlerEle.querySelector('div')!;
    childEle.dispatchEvent(
      new ErrorEvent('error', {
        error: new Error(),
        message: 'error msg',
        bubbles: true,
      })
    );
    await aTimeout(0);
    expect(
      errorHandlerEle.shadowRoot?.querySelector('pre')?.textContent
    ).toMatch('error msg');
  });

  // The second dispatch doesn't seem to trigger the event handler in JSDOM.
  // TODO(weiweilin): investigate why the second dispatch doesn't trigger the
  // event handler.
  // eslint-disable-next-line jest/no-disabled-tests
  test.skip('should update error message when received a new error event', async () => {
    const errorHandlerEle = await fixture<ErrorHandlerTestDefaultElement>(html`
      <milo-error-handler-test-default>
        <div></div>
      </milo-error-handler-test-default>
    `);
    const childEle = errorHandlerEle.querySelector('div')!;
    childEle.dispatchEvent(
      new ErrorEvent('error', {
        error: new Error(),
        message: 'error msg',
        bubbles: true,
      })
    );
    await aTimeout(0);
    expect(
      errorHandlerEle.shadowRoot?.querySelector('pre')?.textContent
    ).toMatch('error msg');
    childEle.dispatchEvent(
      new ErrorEvent('error', {
        error: new Error(),
        message: 'error msg 2',
        bubbles: true,
      })
    );
    await aTimeout(0);
    expect(
      errorHandlerEle.shadowRoot?.querySelector('pre')?.textContent
    ).toMatch('error msg 2');
  });

  test('should render the original content when onErrorRender returns false', async () => {
    const errorHandlerEle =
      await fixture<ErrorHandlerTestOnErrorReturnsPropElement>(html`
        <milo-error-handler-test-on-error-returns-prop>
          <div></div>
        </milo-error-handler-test-on-error-returns-prop>
      `);
    const childEle = errorHandlerEle.querySelector('div')!;
    childEle.dispatchEvent(
      new ErrorEvent('error', {
        error: new Error(),
        message: '',
        bubbles: true,
      })
    );
    await aTimeout(0);
    expect(errorHandlerEle.shadowRoot!.querySelector('pre')).toBeNull();
  });
});

describe('reportError', () => {
  test('should dispatch an error event when the fn throws', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = jest.spyOn(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');

    expect(
      reportError(div, () => {
        throw err;
      })
    ).toThrow(err);
    expect(dispatchEventStub.mock.calls.length).toStrictEqual(1);
    const event = dispatchEventStub.mock.lastCall?.[0];
    expect(event).toBeInstanceOf(ErrorEvent);
    expect(event?.bubbles).toBeTruthy();
    expect(event?.composed).toBeTruthy();
    expect((event as ErrorEvent).error).toStrictEqual(err);
    expect((event as ErrorEvent).message).toStrictEqual('err msg');
  });

  test('should not throw the error when fallbackFn is provided', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = jest.spyOn(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');

    reportError(
      div,
      () => {
        throw err;
      },
      () => {}
    )();
    expect(dispatchEventStub.mock.calls.length).toStrictEqual(1);
    const event = dispatchEventStub.mock.lastCall?.[0];
    expect(event).toBeInstanceOf(ErrorEvent);
    expect(event?.bubbles).toBeTruthy();
    expect(event?.composed).toBeTruthy();
    expect((event as ErrorEvent).error).toStrictEqual(err);
    expect((event as ErrorEvent).message).toStrictEqual('err msg');
  });

  test('should still dispatch the original error event when fallbackFn throws', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = jest.spyOn(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    class FallbackErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');
    const fallbackErr = new FallbackErrorClass('fallback err msg');

    expect(
      reportError(
        div,
        () => {
          throw err;
        },
        () => {
          throw fallbackErr;
        }
      )
    ).toThrow(fallbackErr);
    expect(dispatchEventStub.mock.calls.length).toStrictEqual(1);
    const event = dispatchEventStub.mock.lastCall?.[0];
    expect(event).toBeInstanceOf(ErrorEvent);
    expect(event?.bubbles).toBeTruthy();
    expect(event?.composed).toBeTruthy();
    expect((event as ErrorEvent).error).toStrictEqual(err);
    expect((event as ErrorEvent).message).toStrictEqual('err msg');
  });
});

describe('reportErrorAsync', () => {
  test('should dispatch an error event when the fn throws', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = jest.spyOn(div, 'dispatchEvent');
    const err = new Error('err msg');
    await expect(
      reportErrorAsync(div, async () => {
        throw err;
      })
    ).rejects.toThrow(err);
    expect(dispatchEventStub.mock.calls.length).toStrictEqual(1);
    const event = dispatchEventStub.mock.lastCall?.[0];
    expect(event).toBeInstanceOf(ErrorEvent);
    expect(event?.bubbles).toBeTruthy();
    expect(event?.composed).toBeTruthy();
    expect((event as ErrorEvent).error).toStrictEqual(err);
    expect((event as ErrorEvent).message).toStrictEqual('err msg');
  });

  test('should dispatch an error event when the fn throws immediately', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = jest.spyOn(div, 'dispatchEvent');
    const err = new Error('err msg');
    await expect(
      reportErrorAsync(div, () => {
        throw err;
      })
    ).rejects.toThrow(err);
    expect(dispatchEventStub.mock.calls.length).toStrictEqual(1);
    const event = dispatchEventStub.mock.lastCall?.[0];
    expect(event).toBeInstanceOf(ErrorEvent);
    expect(event?.bubbles).toBeTruthy();
    expect(event?.composed).toBeTruthy();
    expect((event as ErrorEvent).error).toStrictEqual(err);
    expect((event as ErrorEvent).message).toStrictEqual('err msg');
  });

  test('should not throw the error when fallbackFn is provided', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = jest.spyOn(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');

    await reportErrorAsync(
      div,
      () => {
        throw err;
      },
      async () => {}
    )();
    expect(dispatchEventStub.mock.calls.length).toStrictEqual(1);
    const event = dispatchEventStub.mock.lastCall?.[0];
    expect(event).toBeInstanceOf(ErrorEvent);
    expect(event?.bubbles).toBeTruthy();
    expect(event?.composed).toBeTruthy();
    expect((event as ErrorEvent).error).toStrictEqual(err);
    expect((event as ErrorEvent).message).toStrictEqual('err msg');
  });

  test('should still dispatch the original error event when fallbackFn throws', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = jest.spyOn(div, 'dispatchEvent');
    const err = new Error('err msg');
    const fallbackErr = new Error('fallback err msg');

    await expect(
      reportErrorAsync(
        div,
        async () => {
          throw err;
        },
        async () => {
          throw fallbackErr;
        }
      )
    ).rejects.toThrow(fallbackErr);
    expect(dispatchEventStub.mock.calls.length).toStrictEqual(1);
    const event = dispatchEventStub.mock.lastCall?.[0];
    expect(event).toBeInstanceOf(ErrorEvent);
    expect(event?.bubbles).toBeTruthy();
    expect(event?.composed).toBeTruthy();
    expect((event as ErrorEvent).error).toStrictEqual(err);
    expect((event as ErrorEvent).message).toStrictEqual('err msg');
  });

  test('should still dispatch the original error event when fallbackFn throws immediately', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = jest.spyOn(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    class FallbackErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');
    const fallbackErr = new FallbackErrorClass('fallback err msg');

    await expect(
      reportErrorAsync(
        div,
        async () => {
          throw err;
        },
        () => {
          throw fallbackErr;
        }
      )
    ).rejects.toThrow(fallbackErr);
    expect(dispatchEventStub.mock.calls.length).toStrictEqual(1);
    const event = dispatchEventStub.mock.lastCall?.[0];
    expect(event).toBeInstanceOf(ErrorEvent);
    expect(event?.bubbles).toBeTruthy();
    expect(event?.composed).toBeTruthy();
    expect((event as ErrorEvent).error).toStrictEqual(err);
    expect((event as ErrorEvent).message).toStrictEqual('err msg');
  });
});

@customElement('milo-error-handler-test-forward-without-msg')
@errorHandler(forwardWithoutMsg)
class ErrorHandlerTestForwardWithoutMsgElement extends LitElement {
  protected render() {
    return html`<slot></slot>`;
  }
}

@customElement('milo-error-handler-test-recover')
@errorHandler((e) => {
  e.stopImmediatePropagation();
  e.preventDefault();
  return false;
})
class ErrorHandlerTestRecoverElement extends LitElement {
  protected render() {
    return html`<slot></slot>`;
  }
}

describe('forwardWithoutMsg', () => {
  test('should dispatch an error event to the parent element with the same error but no message', async () => {
    const parentEle = await fixture(html`
      <div>
        <milo-error-handler-test-forward-without-msg>
          <div></div>
        </milo-error-handler-test-forward-without-msg>
      </div>
    `);
    const parentDispatchEventStub = jest
      .spyOn(parentEle, 'dispatchEvent')
      .mockImplementation(() => true);
    const errorHandlerEle =
      parentEle.querySelector<ErrorHandlerTestForwardWithoutMsgElement>(
        'milo-error-handler-test-forward-without-msg'
      )!;
    const err = new Error('error msg');
    const childEle = errorHandlerEle.querySelector('div')!;
    childEle.dispatchEvent(
      new ErrorEvent('error', {
        error: err,
        message: 'error msg',
        bubbles: true,
      })
    );
    await aTimeout(0);
    expect(
      errorHandlerEle.shadowRoot?.querySelector('pre')?.textContent
    ).toMatch('error msg');
    expect(parentDispatchEventStub.mock.calls.length).toStrictEqual(1);
    const event = parentDispatchEventStub.mock.lastCall?.[0];
    expect(event).toBeInstanceOf(ErrorEvent);
    expect(event?.bubbles).toBeTruthy();
    expect(event?.composed).toBeTruthy();
    expect((event as ErrorEvent).error).toStrictEqual(err);
    expect((event as ErrorEvent).message).toStrictEqual('');
  });

  test('should recover from the error if e.preventDefault() is called', async () => {
    const parentEle = await fixture(html`
      <milo-error-handler-test-recover>
        <milo-error-handler-test-forward-without-msg>
          <div></div>
        </milo-error-handler-test-forward-without-msg>
      </milo-error-handler-test-recover>
    `);
    const errorHandlerEle =
      parentEle.querySelector<ErrorHandlerTestForwardWithoutMsgElement>(
        'milo-error-handler-test-forward-without-msg'
      )!;
    const err = new Error('error msg');
    const childEle = errorHandlerEle.querySelector('div')!;
    childEle.dispatchEvent(
      new ErrorEvent('error', {
        error: err,
        message: 'error msg',
        bubbles: true,
        cancelable: true,
      })
    );
    await aTimeout(0);
    expect(errorHandlerEle.shadowRoot?.querySelector('pre')).toBeNull();
  });

  test('can recover from the error even when the error is forwarded multiple times', async () => {
    const parentEle = await fixture<ErrorHandlerTestRecoverElement>(html`
      <milo-error-handler-test-recover>
        <milo-error-handler-test-forward-without-msg id="outer">
          <div>
            <milo-error-handler-test-forward-without-msg id="inner">
              <div id="child"></div>
            </milo-error-handler-test-forward-without-msg>
          </div>
        </milo-error-handler-test-forward-without-msg>
      </milo-error-handler-test-recover>
    `);
    const outerErrorHandlerEle =
      parentEle.querySelector<ErrorHandlerTestForwardWithoutMsgElement>(
        '#outer'
      )!;
    const innerErrorHandlerEle =
      parentEle.querySelector<ErrorHandlerTestForwardWithoutMsgElement>(
        '#inner'
      )!;
    const err = new Error('error msg');
    const childEle = innerErrorHandlerEle.querySelector('#child')!;
    childEle.dispatchEvent(
      new ErrorEvent('error', {
        error: err,
        message: 'error msg',
        bubbles: true,
        cancelable: true,
      })
    );
    await aTimeout(0);
    expect(outerErrorHandlerEle.shadowRoot?.querySelector('pre')).toBeNull();
    expect(innerErrorHandlerEle.shadowRoot?.querySelector('pre')).toBeNull();
  });
});
