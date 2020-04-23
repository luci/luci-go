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
import '@material/mwc-checkbox';
import { Checkbox } from '@material/mwc-checkbox';
import '@material/mwc-formfield';
import { css, customElement, html } from 'lit-element';
import { autorun, observable } from 'mobx';

export interface TestFilter {
  showExpected: boolean;
  showExonerated: boolean;
}

/**
 * An element that let the user toggles filter for the tests.
 * Notifies the parent element via onFilterChanged callback when the filter is
 * changed.
 */
@customElement('tr-test-filter')
export class TestFilterElement extends MobxLitElement {
  onFilterChanged: (filter: TestFilter) => void = () => {};

  @observable.ref showExpected = false;
  @observable.ref showExonerated = false;

  private disposer = () => {};
  connectedCallback() {
    super.connectedCallback();
    this.disposer = autorun(
      () => this.onFilterChanged({showExpected: this.showExpected, showExonerated: this.showExonerated}),
    );
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    this.disposer();
  }

  protected render() {
    return html`
      <mwc-formfield label="Unexpected">
        <mwc-checkbox
          checked
          disabled
        ></mwc-checkbox>
      </mwc-formfield>
      <mwc-formfield label="Expected">
        <mwc-checkbox
          ?checked=${this.showExpected}
          @change=${(v: MouseEvent) => this.showExpected = (v.target as Checkbox).checked}
        ></mwc-checkbox>
      </mwc-formfield>
      <mwc-formfield label="Exonerated">
        <mwc-checkbox
          ?checked=${this.showExonerated}
          @change=${(v: MouseEvent) => this.showExonerated = (v.target as Checkbox).checked}
        ></mwc-checkbox>
      </mwc-formfield>
    `;
  }

  static styles = css`
    :host {
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      padding: 5px 0;
    }
    mwc-formfield > mwc-checkbox {
      margin-right: -10px;
    }
  `;
}
