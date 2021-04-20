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

import { aTimeout, fixture, fixtureCleanup, html } from '@open-wc/testing/index-no-side-effects';
import { assert } from 'chai';
import { customElement, LitElement } from 'lit-element';

import './error_handler';
import { ErrorHandlerElement, reportError, reportErrorAsync } from './error_handler';

@customElement('milo-error-handler-test')
export class ErrorHandlerTestElement extends LitElement {
  protected render = reportError.bind(this)(() => {
    throw new Error('error msg');
  });
}

@customElement('milo-error-handler-test-with-fallback-fn')
export class ErrorHandlerTestWithFallbackFnElement extends LitElement {
  protected render = reportError.bind(this)(
    () => {
      throw new Error('error msg');
    },
    () => html`fallback`
  );
}

@customElement('milo-error-handler-test-with-throwing-fallback-fn')
export class ErrorHandlerTestWithThrowingFallbackFnElement extends LitElement {
  protected render = reportError.bind(this)(
    () => {
      throw new Error('error msg');
    },
    () => {
      throw new Error('error msg from fallbackFn');
    }
  );
}

@customElement('milo-error-handler-test-nested')
export class ErrorHandlerTestNestedElement extends LitElement {
  protected render() {
    return html`<milo-error-handler-test></milo-error-handler-test>`;
  }
}

describe('reportError', () => {
  it('should render error message', async () => {
    after(fixtureCleanup);
    const errorHandlerEle = await fixture<ErrorHandlerElement>(html`
      <milo-error-handler>
        <milo-error-handler-test></milo-error-handler-test>
      </milo-error-handler>
    `);
    assert.strictEqual(errorHandlerEle.errorMsg, 'Error: error msg');
  });

  it('should dispatch error event', async () => {
    after(fixtureCleanup);
    let msg = '';
    await fixture<ErrorHandlerElement>(html`
      <div
        @error=${(e: ErrorEvent) => {
          msg = e.message;
          e.stopPropagation();
        }}
      >
        <milo-error-handler-test></milo-error-handler-test>
      </div>
    `);
    assert.strictEqual(msg, 'Error: error msg');
  });

  it('should not render error message when intercept fn returns null', async () => {
    after(fixtureCleanup);
    const errorHandlerEle = await fixture<ErrorHandlerElement>(html`
      <milo-error-handler .intercept=${() => null}>
        <milo-error-handler-test></milo-error-handler-test>
      </milo-error-handler>
    `);
    assert.strictEqual(errorHandlerEle.errorMsg, null);
  });

  it('should render error message when intercept fn returns an error event', async () => {
    after(fixtureCleanup);
    const errorHandlerEle = await fixture<ErrorHandlerElement>(html`
      <milo-error-handler .intercept=${() => new ErrorEvent('error')}>
        <milo-error-handler-test></milo-error-handler-test>
      </milo-error-handler>
    `);

    // Null means the there's no error.
    // '' means the error message is empty.
    assert.notStrictEqual(errorHandlerEle.errorMsg, null);
    assert.strictEqual(errorHandlerEle.errorMsg, '');
  });

  it('should render error message from the original fn', async () => {
    after(fixtureCleanup);
    const errorHandlerEle = await fixture<ErrorHandlerElement>(html`
      <milo-error-handler>
        <milo-error-handler-test-with-throwing-fallback-fn></milo-error-handler-test-with-throwing-fallback-fn>
      </milo-error-handler>
    `);
    assert.strictEqual(errorHandlerEle.errorMsg, 'Error: error msg');
  });

  it('should be able to collect errors originated from shadow DOM.', async () => {
    after(fixtureCleanup);
    const errorHandlerEle = await fixture<ErrorHandlerElement>(html`
      <milo-error-handler>
        <milo-error-handler-test-nested></milo-error-handler-test-nested>
      </milo-error-handler>
    `);
    assert.strictEqual(errorHandlerEle.errorMsg, 'Error: error msg');
  });

  it('only the closest ancestor should handle the error', async () => {
    after(fixtureCleanup);
    const outerErrorHandler = await fixture<ErrorHandlerElement>(html`
      <milo-error-handler>
        <milo-error-handler>
          <milo-error-handler-test></milo-error-handler-test>
        </milo-error-handler>
      </milo-error-handler>
    `);
    const innerErrorHandler = outerErrorHandler.querySelector<ErrorHandlerElement>('milo-error-handler')!;
    assert.strictEqual(outerErrorHandler.errorMsg, null);
    assert.strictEqual(innerErrorHandler.errorMsg, 'Error: error msg');
  });

  it('should fallback to fallbackFn on error', async () => {
    after(fixtureCleanup);
    const errorHandlerEle = await fixture<ErrorHandlerElement>(html`
      <milo-error-handler>
        <milo-error-handler-test-with-fallback-fn></milo-error-handler-test-with-fallback-fn>
      </milo-error-handler>
    `);

    const withFallbackFnEle = errorHandlerEle.querySelector<ErrorHandlerTestWithFallbackFnElement>(
      'milo-error-handler-test-with-fallback-fn'
    )!;
    assert.strictEqual(withFallbackFnEle.shadowRoot?.getRootNode().textContent, 'fallback');
  });

  it('should rethrow the error', async () => {
    const ele = document.createElement('div');
    const err = new Error('test');
    const fn = reportError.bind(ele)(() => {
      throw err;
    });

    try {
      fn();
      assert.fail("fn should've thrown an error");
    } catch (e) {
      assert.strictEqual(e, err);
    }
  });

  it('should not rethrow the error when fallbackFn is provided', async () => {
    const ele = document.createElement('div');
    const fn = reportError.bind(ele)(
      () => {
        throw new Error('test');
      },
      () => ''
    );

    assert.doesNotThrow(fn);
  });

  it('should rethrow the error when fallbackFn thrown an error', async () => {
    const ele = document.createElement('div');
    const err = new Error('from fallbackFn');
    const fn = reportError.bind(ele)(
      () => {
        throw new Error('test');
      },
      () => {
        throw err;
      }
    );

    try {
      fn();
      assert.fail("fn should've thrown an error");
    } catch (e) {
      assert.strictEqual(e, err);
    }
  });
});

describe('reportErrorAsync', () => {
  let errorHandlerEle: ErrorHandlerElement;
  let errorSrcEle: HTMLDivElement;
  before(async () => {
    errorHandlerEle = await fixture<ErrorHandlerElement>(html`
      <milo-error-handler>
        <div></div>
      </milo-error-handler>
    `);
    errorSrcEle = errorHandlerEle.querySelector<HTMLDivElement>('div')!;
  });
  after(fixtureCleanup);

  it('should render error message', async () => {
    const err = new Error('test');
    try {
      await reportErrorAsync.bind(errorSrcEle)(async () => {
        await aTimeout(0);
        throw err;
      })();
      assert.fail("should've thrown an error");
    } catch (e) {
      assert.strictEqual(e, err);
    }
    assert.strictEqual(errorHandlerEle.errorMsg, 'Error: test');
  });

  it('should render error message when fn throws immediately', async () => {
    const err = new Error('test');
    try {
      await reportErrorAsync.bind(errorSrcEle)(() => {
        throw err;
      })();
      assert.fail("should've thrown an error");
    } catch (e) {
      assert.strictEqual(e, err);
    }
    assert.strictEqual(errorHandlerEle.errorMsg, 'Error: test');
  });

  it('should render error message from the original fn', async () => {
    const err = new Error('test from fallbackFn');
    try {
      await reportErrorAsync.bind(errorSrcEle)(
        async () => {
          throw new Error('test');
        },
        async () => {
          await aTimeout(0);
          throw err;
        }
      )();
      assert.fail("should've thrown an error");
    } catch (e) {
      assert.strictEqual(e, err);
    }
    assert.strictEqual(errorHandlerEle.errorMsg, 'Error: test');
  });

  it('should return the value from fallbackFn', async () => {
    const err = new Error('test');
    const ret = await reportErrorAsync.bind(errorSrcEle)(
      async () => {
        throw err;
      },
      async () => 'fallback'
    )();
    assert.strictEqual(ret, 'fallback');
  });
});
