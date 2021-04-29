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
import { css, customElement, LitElement, property } from 'lit-element';
import * as sinon from 'sinon';

import {
  EnterViewNotifier,
  enterViewObserver,
  lazyRendering,
  OnEnterView,
  RenderPlaceHolder,
} from './enter_view_observer';

@customElement('milo-enter-view-observer-test-entry')
@enterViewObserver((e: EnterViewObserverTestEntryElement) => new EnterViewNotifier({ root: e.parentElement }))
class EnterViewObserverTestEntryElement extends LitElement implements OnEnterView {
  @property() onEnterCallCount = 0;

  onEnterView() {
    this.onEnterCallCount++;
  }

  protected render() {
    return html`content`;
  }

  static styles = css`
    :host {
      display: block;
      height: 10px;
    }
  `;
}

@customElement('milo-enter-view-observer-notifier-test-entry')
@enterViewObserver((e: EnterViewObserverNotifierTestEntryElement) => e.notifier)
class EnterViewObserverNotifierTestEntryElement extends LitElement implements OnEnterView {
  notifier!: EnterViewNotifier;
  @property() onEnterCallCount = 0;

  onEnterView() {
    this.onEnterCallCount++;
  }

  protected render() {
    return html`content`;
  }

  static styles = css`
    :host {
      display: block;
      height: 10px;
    }
  `;
}

describe('enterViewObserver', () => {
  let listView: HTMLDivElement;
  let entries: NodeListOf<EnterViewObserverTestEntryElement>;

  beforeEach(async () => {
    listView = await fixture<HTMLDivElement>(html`
      <div style="height: 100px; overflow-y: auto;">
        ${new Array(100)
          .fill(0)
          .map(() => html`<milo-enter-view-observer-test-entry></milo-enter-view-observer-test-entry>`)}
      </div>
    `);
    entries = listView.querySelectorAll<EnterViewObserverTestEntryElement>('milo-enter-view-observer-test-entry');
  });
  afterEach(fixtureCleanup);

  it('should notify entries in the view.', async () => {
    await aTimeout(0);
    entries.forEach((entry, i) => {
      assert.equal(entry.onEnterCallCount, i <= 10 ? 1 : 0);
    });
  });

  it('should notify new entries scrolls into the view.', async () => {
    await aTimeout(0);
    listView.scrollBy(0, 50);
    await aTimeout(20);

    entries.forEach((entry, i) => {
      assert.equal(entry.onEnterCallCount, i <= 15 ? 1 : 0);
    });
  });

  it('should re-notify old entries when scrolling back and forth.', async () => {
    await aTimeout(0);
    listView.scrollBy(0, 50);
    await aTimeout(20);
    listView.scrollBy(0, -50);
    await aTimeout(20);

    entries.forEach((entry, i) => {
      assert.equal(entry.onEnterCallCount, i <= 15 ? 1 : 0);
    });
  });

  it('different instances can have different notifiers', async () => {
    const entry1 = document.createElement(
      'milo-enter-view-observer-notifier-test-entry'
    ) as EnterViewObserverNotifierTestEntryElement;
    const entry2 = document.createElement(
      'milo-enter-view-observer-notifier-test-entry'
    ) as EnterViewObserverNotifierTestEntryElement;
    const notifier1 = new EnterViewNotifier();
    const notifier2 = new EnterViewNotifier();
    const notifierStub1 = sinon.stub(notifier1);
    const notifierStub2 = sinon.stub(notifier2);

    entry1.notifier = notifier1;
    entry2.notifier = notifier2;

    entry1.connectedCallback();
    entry2.connectedCallback();
    entry1.disconnectedCallback();
    entry2.disconnectedCallback();

    assert.strictEqual(notifierStub1.observe.callCount, 1);
    assert.strictEqual(notifierStub1.observe.getCall(0).args[0], entry1);
    assert.strictEqual(notifierStub2.observe.callCount, 1);
    assert.strictEqual(notifierStub2.observe.getCall(0).args[0], entry2);

    assert.strictEqual(notifierStub1.unobserve.callCount, 1);
    assert.strictEqual(notifierStub1.unobserve.getCall(0).args[0], entry1);
    assert.strictEqual(notifierStub2.unobserve.callCount, 1);
    assert.strictEqual(notifierStub2.unobserve.getCall(0).args[0], entry2);
  });
});

@customElement('milo-lazy-rendering-test-entry')
@lazyRendering((e: LazyRenderingElement) => new EnterViewNotifier({ root: e.parentElement }))
class LazyRenderingElement extends LitElement implements RenderPlaceHolder {
  renderPlaceHolder() {
    return html`placeholder`;
  }

  protected render() {
    return html`content`;
  }

  static styles = css`
    :host {
      display: block;
      height: 10px;
    }
  `;
}

describe('lazyRendering', () => {
  let listView: HTMLDivElement;
  let entries: NodeListOf<LazyRenderingElement>;

  beforeEach(async () => {
    listView = await fixture<HTMLDivElement>(html`
      <div style="height: 100px; overflow-y: auto;">
        ${new Array(100).fill(0).map(() => html`<milo-lazy-rendering-test-entry></milo-lazy-rendering-test-entry>`)}
      </div>
    `);
    entries = listView.querySelectorAll<LazyRenderingElement>('milo-lazy-rendering-test-entry');
  });
  afterEach(fixtureCleanup);

  it('should only render content for elements entered the view.', async () => {
    await aTimeout(0);
    entries.forEach((entry, i) => {
      assert.equal(entry.shadowRoot!.textContent, i <= 10 ? 'content' : 'placeholder', 'error: ' + i);
    });
  });

  it('should work with scrolling', async () => {
    await aTimeout(0);
    listView.scrollBy(0, 50);
    await aTimeout(20);

    entries.forEach((entry, i) => {
      assert.equal(entry.shadowRoot!.textContent, i <= 15 ? 'content' : 'placeholder', '2 error: ' + i);
    });
  });
});
