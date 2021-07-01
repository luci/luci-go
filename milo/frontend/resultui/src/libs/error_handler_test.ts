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
import { aTimeout, fixture, fixtureCleanup, html } from '@open-wc/testing/index-no-side-effects';
import { assert } from 'chai';
import { customElement, LitElement } from 'lit-element';
import * as sinon from 'sinon';

import './error_handler';
import { errorHandler, forwardWithoutMsg, reportError, reportErrorAsync } from './error_handler';

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
  it('should render error message by default', async () => {
    after(fixtureCleanup);
    const errorHandlerEle = await fixture<ErrorHandlerTestDefaultElement>(html`
      <milo-error-handler-test-default>
        <div></div>
      </milo-error-handler-test-default>
    `);
    const childEle = errorHandlerEle.querySelector('div')!;
    childEle.dispatchEvent(new ErrorEvent('error', { error: new Error(), message: 'error msg', bubbles: true }));
    await aTimeout(0);
    assert.include(errorHandlerEle.shadowRoot?.querySelector('pre')?.textContent, 'error msg');
  });

  it('should update error message when received a new error event', async () => {
    after(fixtureCleanup);
    const errorHandlerEle = await fixture<ErrorHandlerTestDefaultElement>(html`
      <milo-error-handler-test-default>
        <div></div>
      </milo-error-handler-test-default>
    `);
    const childEle = errorHandlerEle.querySelector('div')!;
    childEle.dispatchEvent(new ErrorEvent('error', { error: new Error(), message: 'error msg', bubbles: true }));
    await aTimeout(0);
    assert.include(errorHandlerEle.shadowRoot?.querySelector('pre')?.textContent, 'error msg');
    childEle.dispatchEvent(new ErrorEvent('error', { error: new Error(), message: 'error msg 2', bubbles: true }));
    await aTimeout(0);
    assert.include(errorHandlerEle.shadowRoot?.querySelector('pre')?.textContent, 'error msg 2');
  });

  it('should render the original content when onErrorRender returns false', async () => {
    after(fixtureCleanup);
    const errorHandlerEle = await fixture<ErrorHandlerTestOnErrorReturnsPropElement>(html`
      <milo-error-handler-test-on-error-returns-prop>
        <div></div>
      </milo-error-handler-test-on-error-returns-prop>
    `);
    const childEle = errorHandlerEle.querySelector('div')!;
    childEle.dispatchEvent(new ErrorEvent('error', { error: new Error(), message: '', bubbles: true }));
    await aTimeout(0);
    assert.strictEqual(errorHandlerEle.shadowRoot!.querySelector('pre'), null);
  });
});

describe('reportError', () => {
  it('should dispatch an error event when the fn throws', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = sinon.stub(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');

    assert.throw(
      reportError(div, () => {
        throw err;
      }),
      SpecialErrorClass
    );
    assert.strictEqual(dispatchEventStub.callCount, 1);
    const event = dispatchEventStub.getCall(0).args[0];
    assert.instanceOf(event, ErrorEvent);
    assert.isTrue(event.bubbles);
    assert.isTrue(event.composed);
    assert.strictEqual((event as ErrorEvent).error, err);
    assert.strictEqual((event as ErrorEvent).message, 'Error: err msg');
  });

  it('should not throw the error when fallbackFn is provided', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = sinon.stub(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');

    reportError(
      div,
      () => {
        throw err;
      },
      () => {}
    )();
    assert.strictEqual(dispatchEventStub.callCount, 1);
    const event = dispatchEventStub.getCall(0).args[0];
    assert.instanceOf(event, ErrorEvent);
    assert.isTrue(event.bubbles);
    assert.isTrue(event.composed);
    assert.strictEqual((event as ErrorEvent).error, err);
    assert.strictEqual((event as ErrorEvent).message, 'Error: err msg');
  });

  it('should still dispatch the original error event when fallbackFn throws', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = sinon.stub(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    class FallbackErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');
    const fallbackErr = new FallbackErrorClass('fallback err msg');

    assert.throw(
      reportError(
        div,
        () => {
          throw err;
        },
        () => {
          throw fallbackErr;
        }
      ),
      FallbackErrorClass
    );
    assert.strictEqual(dispatchEventStub.callCount, 1);
    const event = dispatchEventStub.getCall(0).args[0];
    assert.instanceOf(event, ErrorEvent);
    assert.isTrue(event.bubbles);
    assert.isTrue(event.composed);
    assert.strictEqual((event as ErrorEvent).error, err);
    assert.strictEqual((event as ErrorEvent).message, 'Error: err msg');
  });
});

describe('reportErrorAsync', () => {
  it('should dispatch an error event when the fn throws', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = sinon.stub(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');
    try {
      await reportErrorAsync(div, async () => {
        throw err;
      })();
      assert.fail("should've thrown an error");
    } catch (e) {
      if (e instanceof SpecialErrorClass) {
        assert.strictEqual(e, err);
      } else {
        throw e;
      }
    }
    assert.strictEqual(dispatchEventStub.callCount, 1);
    const event = dispatchEventStub.getCall(0).args[0];
    assert.instanceOf(event, ErrorEvent);
    assert.isTrue(event.bubbles);
    assert.isTrue(event.composed);
    assert.strictEqual((event as ErrorEvent).error, err);
    assert.strictEqual((event as ErrorEvent).message, 'Error: err msg');
  });

  it('should dispatch an error event when the fn throws immediately', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = sinon.stub(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');
    try {
      await reportErrorAsync(div, () => {
        throw err;
      })();
      assert.fail("should've thrown an error");
    } catch (e) {
      if (e instanceof SpecialErrorClass) {
        assert.strictEqual(e, err);
      } else {
        throw e;
      }
    }
    assert.strictEqual(dispatchEventStub.callCount, 1);
    const event = dispatchEventStub.getCall(0).args[0];
    assert.instanceOf(event, ErrorEvent);
    assert.isTrue(event.bubbles);
    assert.isTrue(event.composed);
    assert.strictEqual((event as ErrorEvent).error, err);
    assert.strictEqual((event as ErrorEvent).message, 'Error: err msg');
  });

  it('should not throw the error when fallbackFn is provided', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = sinon.stub(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');

    await reportErrorAsync(
      div,
      () => {
        throw err;
      },
      async () => {}
    )();
    assert.strictEqual(dispatchEventStub.callCount, 1);
    const event = dispatchEventStub.getCall(0).args[0];
    assert.instanceOf(event, ErrorEvent);
    assert.isTrue(event.bubbles);
    assert.isTrue(event.composed);
    assert.strictEqual((event as ErrorEvent).error, err);
    assert.strictEqual((event as ErrorEvent).message, 'Error: err msg');
  });

  it('should still dispatch the original error event when fallbackFn throws', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = sinon.stub(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    class FallbackErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');
    const fallbackErr = new FallbackErrorClass('fallback err msg');

    try {
      await reportErrorAsync(
        div,
        async () => {
          throw err;
        },
        async () => {
          throw fallbackErr;
        }
      )();
      assert.fail("should've thrown an error");
    } catch (e) {
      if (e instanceof FallbackErrorClass) {
        assert.strictEqual(e, fallbackErr);
      } else {
        throw e;
      }
    }
    assert.strictEqual(dispatchEventStub.callCount, 1);
    const event = dispatchEventStub.getCall(0).args[0];
    assert.instanceOf(event, ErrorEvent);
    assert.isTrue(event.bubbles);
    assert.isTrue(event.composed);
    assert.strictEqual((event as ErrorEvent).error, err);
    assert.strictEqual((event as ErrorEvent).message, 'Error: err msg');
  });

  it('should still dispatch the original error event when fallbackFn throws immediately', async () => {
    const div = document.createElement('div');
    const dispatchEventStub = sinon.stub(div, 'dispatchEvent');
    class SpecialErrorClass extends Error {}
    class FallbackErrorClass extends Error {}
    const err = new SpecialErrorClass('err msg');
    const fallbackErr = new FallbackErrorClass('fallback err msg');

    try {
      await reportErrorAsync(
        div,
        async () => {
          throw err;
        },
        () => {
          throw fallbackErr;
        }
      )();
      assert.fail("should've thrown an error");
    } catch (e) {
      if (e instanceof FallbackErrorClass) {
        assert.strictEqual(e, fallbackErr);
      } else {
        throw e;
      }
    }
    assert.strictEqual(dispatchEventStub.callCount, 1);
    const event = dispatchEventStub.getCall(0).args[0];
    assert.instanceOf(event, ErrorEvent);
    assert.isTrue(event.bubbles);
    assert.isTrue(event.composed);
    assert.strictEqual((event as ErrorEvent).error, err);
    assert.strictEqual((event as ErrorEvent).message, 'Error: err msg');
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
  it('should dispatch an error event to the parent element with the same error but no message', async () => {
    after(fixtureCleanup);
    const parentEle = await fixture(html`
      <div>
        <milo-error-handler-test-forward-without-msg>
          <div></div>
        </milo-error-handler-test-forward-without-msg>
      </div>
    `);
    const parentDispatchEventStub = sinon.stub(parentEle, 'dispatchEvent');
    const errorHandlerEle = parentEle.querySelector<ErrorHandlerTestForwardWithoutMsgElement>(
      'milo-error-handler-test-forward-without-msg'
    )!;
    const err = new Error('error msg');
    const childEle = errorHandlerEle.querySelector('div')!;
    childEle.dispatchEvent(new ErrorEvent('error', { error: err, message: 'error msg', bubbles: true }));
    await aTimeout(0);
    assert.include(errorHandlerEle.shadowRoot?.querySelector('pre')?.textContent, 'error msg');
    assert.strictEqual(parentDispatchEventStub.callCount, 1);
    const event = parentDispatchEventStub.getCall(0).args[0];
    assert.instanceOf(event, ErrorEvent);
    assert.isTrue(event.bubbles);
    assert.isTrue(event.composed);
    assert.strictEqual((event as ErrorEvent).error, err);
    assert.strictEqual((event as ErrorEvent).message, '');
  });

  it('should recover from the error if e.preventDefault() is called', async () => {
    after(fixtureCleanup);
    const parentEle = await fixture(html`
      <milo-error-handler-test-recover>
        <milo-error-handler-test-forward-without-msg>
          <div></div>
        </milo-error-handler-test-forward-without-msg>
      </milo-error-handler-test-recover>
    `);
    const errorHandlerEle = parentEle.querySelector<ErrorHandlerTestForwardWithoutMsgElement>(
      'milo-error-handler-test-forward-without-msg'
    )!;
    const err = new Error('error msg');
    const childEle = errorHandlerEle.querySelector('div')!;
    childEle.dispatchEvent(
      new ErrorEvent('error', { error: err, message: 'error msg', bubbles: true, cancelable: true })
    );
    await aTimeout(0);
    assert.strictEqual(errorHandlerEle.shadowRoot?.querySelector('pre'), null);
  });

  it('can recover from the error even when the error is forwarded multiple times', async () => {
    after(fixtureCleanup);
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
    const outerErrorHandlerEle = parentEle.querySelector<ErrorHandlerTestForwardWithoutMsgElement>('#outer')!;
    const innerErrorHandlerEle = parentEle.querySelector<ErrorHandlerTestForwardWithoutMsgElement>('#inner')!;
    const err = new Error('error msg');
    const childEle = innerErrorHandlerEle.querySelector('#child')!;
    childEle.dispatchEvent(
      new ErrorEvent('error', { error: err, message: 'error msg', bubbles: true, cancelable: true })
    );
    await aTimeout(0);
    assert.strictEqual(outerErrorHandlerEle.shadowRoot?.querySelector('pre'), null);
    assert.strictEqual(innerErrorHandlerEle.shadowRoot?.querySelector('pre'), null);
  });
});
