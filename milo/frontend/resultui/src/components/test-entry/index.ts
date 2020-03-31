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

import { MobxLitElement } from '@adobe/lit-mobx';
import '@material/mwc-icon';
import { css, customElement, html } from 'lit-element';
import { classMap } from 'lit-html/directives/class-map';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
// TODO(weiweilin): investigate the impact on the bundle size after
// optimization and decide whether to use lodash or not.
import takeWhile from 'lodash-es/takeWhile';
import { computed, observable } from 'mobx';

import { ID_SEG_REGEX, ReadonlyTest } from '../../models/test_node';
import '../copy_to_clipboard';


/**
 * Renders an expandable entry of the given test.
 * The common test ID prefix between the given test and the previous test are
 * greyed out to create a tree-like hierarchy when rendered in a list.
 * Results and exonerations are grouped into variants.
 */
@customElement('tr-test-entry')
export class TestEntryElement extends MobxLitElement {
  @observable.ref test?: ReadonlyTest;
  @observable.ref rootName = '';
  @observable.ref prevTestId = '';

  @observable.ref expanded = false;
  // Always render the children once it was expanded so the children's state
  // don't get reset after the node is collapsed.
  @observable.ref private wasExpanded = false;

  @computed
  private get hasUnexpected() {
    return this.test!.variants.some((v) => v.results.some((r) => !r.expected));
  }

  /**
   * Common prefix between this.test.id and this.prevTestId.
   */
  @computed
  private get commonTestIdPrefix() {
    const prevSegs = this.prevTestId.match(ID_SEG_REGEX)!;
    const currentSegs = this.test!.id.match(ID_SEG_REGEX)!;
    return takeWhile(prevSegs, (seg, i) => currentSegs[i] === seg).join('');
  }

  protected render() {
    return html`
      <div class=${classMap({unexpected: this.hasUnexpected})}>
        <div
          class="expandable-header"
          @click=${() => {
            this.expanded = !this.expanded;
            this.wasExpanded = true;
          }}
        >
          <mwc-icon id="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
          <div id="header" class="one-line-content">
            <mwc-icon
              id="expectancy-indicator"
              class=${classMap({unexpected: this.hasUnexpected})}
            >${this.hasUnexpected ?  'error': 'check'}</mwc-icon>
            <div id="test-identifier">
              <span class="light">${this.commonTestIdPrefix.slice(this.rootName.length)}</span>${this.test!.id.slice(this.commonTestIdPrefix.length)}
              <tr-copy-to-clipboard
                .textToCopy=${this.test!.id}
                @click=${(e: Event) => e.stopPropagation()}
                title="copy test ID to clipboard"
              ></tr-copy-to-clipboard>
            </div>
          </div>
        </div>
        <div id="body">
          <div id="content-ruler"></div>
          <div id="content" style=${styleMap({display: this.expanded ? '' : 'none'})}>
            ${repeat(this.wasExpanded ? this.test!.variants : [], (_variant, i) => i, (v) => html`
            <!-- TODO(weiweilin): implement <tr-variant-entry> -->
            <tr-variant-entry .variant=${v} .expanded=${this.test!.variants.length === 1}></tr-variant-entry>
            `)}
          </div>
        </div>
      </div>
    `;
  }

  static styles = css`
    #body {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-gap: 5px;
    }
    #content-ruler {
      border-left: 1px solid #DDDDDD;
      width: 0px;
      margin-left: 11.5px;
    }
    .light {
      color: grey;
    }

    .expandable-header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: 24px;
      letter-spacing: 0.15px;
      grid-gap: 5px;
      cursor: pointer;
      user-select: none;
    }
    .expandable-header .expand-toggle {
      grid-row: 1;
      grid-column: 1;
    }
    .expandable-header .one-line-content {
      grid-row: 1;
      grid-column: 2;
      font-size: 16px;
      line-height: 24px;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }

    #header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: 24px;
      grid-gap: 5px;
    }

    #expectancy-indicator {
      color: #33ac71;
      grid-row: 1;
      grid-column: 1;
    }
    #expectancy-indicator.unexpected {
      color: #d23f31;
    }

    #test-identifier {
      grid-row: 1;
      grid-column: 2;
      font-size: 16px;
      line-height: 24px;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }
  `;
}
