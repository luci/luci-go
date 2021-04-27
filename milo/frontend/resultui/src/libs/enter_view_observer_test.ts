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

import { EnterViewNotifier, enterViewObserver, OnEnterView } from './enter_view_observer';

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

describe('enterViewObserver', () => {
  let listView: HTMLDivElement;
  let entries: NodeListOf<EnterViewObserverTestEntryElement>;

  before(async () => {
    listView = await fixture<HTMLDivElement>(html`
      <div style="height: 100px; overflow-y: auto;">
        ${new Array(100)
          .fill(0)
          .map(() => html`<milo-enter-view-observer-test-entry></milo-enter-view-observer-test-entry>`)}
      </div>
    `);
    entries = listView.querySelectorAll<EnterViewObserverTestEntryElement>('milo-enter-view-observer-test-entry');
  });
  after(fixtureCleanup);

  it('should notify entries in the view.', async () => {
    await aTimeout(0);
    entries.forEach((entry, i) => {
      assert.equal(entry.onEnterCallCount, i <= 10 ? 1 : 0);
    });
  });

  it('should notify new entries scrolls into the view.', async () => {
    listView.scrollBy(0, 50);
    await aTimeout(10);

    entries.forEach((entry, i) => {
      assert.equal(entry.onEnterCallCount, i <= 15 ? 1 : 0);
    });
  });

  it('should re-notify old entries when scrolling back and forth.', async () => {
    listView.scrollBy(0, -50);
    await aTimeout(10);

    entries.forEach((entry, i) => {
      assert.equal(entry.onEnterCallCount, i <= 15 ? 1 : 0);
    });
  });
});
