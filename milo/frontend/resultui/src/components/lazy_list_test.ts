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

import { aTimeout } from '@open-wc/testing-helpers/index-no-side-effects';
import { fixture, fixtureCleanup, html } from '@open-wc/testing/index-no-side-effects.js';
import { assert } from 'chai';
import { css, customElement, LitElement } from 'lit-element';

import './lazy_list';
import { LazyListElement, OnEnterList } from './lazy_list';

@customElement('milo-lazy-list-test-entry')
class LazyListTestEntryElement extends LitElement implements OnEnterList {
  onEnterCallCount = 0;

  onEnterList() {
    this.onEnterCallCount++;
  }

  protected render() {
    return html`
      <div id="placeholder">A</div>
    `;
  }

  static styles = css`
    #placeholder {
      height: 10px;
    }
  `;
}

describe('lazy_list_test', () => {
  describe('lazy_list', async () => {
    after(fixtureCleanup);

    const lazyList = await fixture<LazyListElement>(html`
      <milo-lazy-list style="height: 100px;">
        ${new Array(100).fill(0).map(() => html`
          <milo-lazy-list-test-entry>
          </milo-lazy-list-test-entry>
        `)}
      </milo-lazy-list>
    `);
    const entries = lazyList.querySelectorAll<LazyListTestEntryElement>('milo-lazy-list-test-entry');


    it('should notify entries in the view.', async () => {
      await aTimeout(LazyListElement.MIN_INTERVAL);
      entries.forEach((entry, i) => {
        assert.equal(entry.onEnterCallCount, i <= 10 ? 1 : 0);
      });
    });

    it('should notify new entries scrolls into the view.', async () => {
      lazyList.scrollTop = 200;
      lazyList.onscroll();

      await aTimeout(LazyListElement.MIN_INTERVAL);
      entries.forEach((entry, i) => {
        assert.equal(entry.onEnterCallCount, i <= 30 ? 1 : 0);
      });
    });

    it('should not re-notify old entries when scrolling back.', async () => {
      lazyList.scrollTop = 0;
      lazyList.onscroll();

      await aTimeout(LazyListElement.MIN_INTERVAL);
      entries.forEach((entry, i) => {
        assert.equal(entry.onEnterCallCount, i <= 30 ? 1 : 0);
      });
    });
  });

  describe('lazy_list with growth', async () => {
    it('should notify entries in the view progressively', async () => {
      after(fixtureCleanup);

      const list = await fixture<LazyListElement>(html`
        <milo-lazy-list .growth=${100} style="height: 100px;">
          ${new Array(100).fill(0).map(() => html`
            <milo-lazy-list-test-entry>
            </milo-lazy-list-test-entry>
          `)}
        </milo-lazy-list>
      `);

      const startTime = Date.now();
      const entries = list.querySelectorAll<LazyListTestEntryElement>('milo-lazy-list-test-entry');

      // Wait half of MIN_INTERVAL to avoid rounding errors when calculating
      // ticks.
      await aTimeout(LazyListElement.MIN_INTERVAL / 2);

      let ticks = 0;
      while (ticks <= 10) {
        ticks = Math.floor((Date.now() - startTime) / LazyListElement.MIN_INTERVAL);
        entries.forEach((entry, i) => {
          // Renders 10 initially, then 10 more per iteration.
          assert.equal(entry.onEnterCallCount, i <= 10 + ticks * 10 ? 1 : 0);
        });
        await aTimeout(LazyListElement.MIN_INTERVAL);
      }
    });
  });
});
