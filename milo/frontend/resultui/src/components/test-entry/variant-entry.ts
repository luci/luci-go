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
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable } from 'mobx';

import { ReadonlyVariant, VariantStatus } from '../../models/test_node';
import './result-entry';


const STATUS_DISPLAY_MAP = {
  [VariantStatus.Exonerated]: 'exonerated',
  [VariantStatus.Expected]: 'expected',
  [VariantStatus.Unexpected]: 'unexpected',
  [VariantStatus.Flaky]: 'flaky',
};

// Just so happens to be the same as STATUS_DISPLAY_MAP.
const STATUS_CLASS_MAP = {
  [VariantStatus.Exonerated]: 'exonerated',
  [VariantStatus.Expected]: 'expected',
  [VariantStatus.Unexpected]: 'unexpected',
  [VariantStatus.Flaky]: 'flaky',
};

/**
 * Renders an expandable entry of the given test variant.
 */
@customElement('tr-variant-entry')
export class VariantEntryElement extends MobxLitElement {
  @observable.ref
  variant?: ReadonlyVariant;

  @observable.ref
  expanded = true;

  @computed
  private get hasSingleChild() {
    return (this.variant!.results.length + this.variant!.exonerations.length) === 1;
  }

  protected render() {
    return html`
      <div>
        <div
          id="entry-header"
          class="expandable-header"
          @click=${() => this.expanded = !this.expanded}
        >
          <mwc-icon class="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
          <span class="one-line-content">
            <span id="status" class=${STATUS_CLASS_MAP[this.variant!.status]}>${STATUS_DISPLAY_MAP[this.variant!.status]}</span>
            variant:
            <span class="light">
              ${Object.entries(this.variant!.variant.def).map(([k, v]) => html`
              <span class="kv-key">${k}</span>
              <span class="kv-value">${v}</span>
              `)}
            </span>
          </span>
        </div>
        <div id="body">
          <div id="content-ruler"></div>
          <div id="content" style=${styleMap({display: this.expanded ? '' : 'none'})}>
            ${repeat(this.variant!.results, (r) => r.resultId, (r, i) => html`
            <tr-result-entry
              .id=${i}
              .testResult=${r}
              .expanded=${this.hasSingleChild || !r.expected}
            ></tr-result-entry>
            `)}
            ${repeat(this.variant!.exonerations, (e) => e.exonerationId, (e, i) => html`
            <!-- TODO(weiweilin): implement exoneration entry -->
            <tr-exoneration-entry
              .id=${this.variant!.results.length + i}
              .testExoneration=${e}
              .expanded=${this.hasSingleChild}
            ></tr-exoneration-entry>
            `)}
          </div>
        </div>
      </div>
    `;
  }

  // TODO(weiweilin): extract the color scheme to a separate stylesheet.
  static styles = css`
    .expandable-header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: 24px;
      grid-gap: 5px;
      cursor: pointer;
    }
    .expandable-header .expand-toggle {
      grid-row: 1;
      grid-column: 1;
    }
    .expandable-header .one-line-content {
      grid-row: 1;
      grid-column: 2;
      line-height: 24px;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }
    #entry-header .one-line-content {
      font-size: 14px;
      letter-spacing: 0.1px;
      font-weight: 500;
    }

    #status.expected {
      color: rgb(51, 172, 113);
    }
    #status.unexpected {
      color: rgb(210, 63, 49);
    }
    #status.flaky {
      color: rgb(210, 63, 49);
    }
    #status.exonerated {
      color: grey;
    }

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

    .kv-key::after {
      content: ':';
    }
    .kv-value::after {
      content: ',';
    }
    .kv-value:last-child::after {
      content: '';
    }
    .light {
      color: grey;
    }
  `;
}
