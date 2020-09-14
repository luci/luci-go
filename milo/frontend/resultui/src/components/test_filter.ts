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
import { css, customElement, html } from 'lit-element';
import { autorun, observable } from 'mobx';

export interface TestFilter {
  showExpected: boolean;
  showExonerated: boolean;
  showFlaky: boolean;
}

/**
 * An element that let the user toggles filter for the tests.
 * Notifies the parent element via onFilterChanged callback when the filter is
 * changed.
 */
@customElement('milo-test-filter')
export class TestFilterElement extends MobxLitElement {
  onFilterChanged: (filter: TestFilter) => void = () => {};

  @observable.ref showExpected = false;
  @observable.ref showExonerated = true;
  @observable.ref showFlaky = true;

  private disposer = () => {};
  connectedCallback() {
    super.connectedCallback();
    this.disposer = autorun(
      () => this.onFilterChanged({
        showExpected: this.showExpected,
        showExonerated: this.showExonerated,
        showFlaky: this.showFlaky,
      }),
    );
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    this.disposer();
  }

  protected render() {
    return html`
      Show:
      <div class="filter">
        <input type="checkbox" id="unexpected" disabled checked>
        <label for="unexpected" style="color: var(--failure-color);">Unexpected</label>
      </div class="filter">
      <div class="filter">
        <input type="checkbox" id="expected" @change=${(v: MouseEvent) => this.showExpected = (v.target as HTMLInputElement).checked} ?checked=${this.showExpected}>
      <label for="expected" style="color: var(--success-color);">Expected</label>
      </div class="filter">
      <div class="filter">
        <input type="checkbox" id="exonerated" @change=${(v: MouseEvent) => this.showExonerated = (v.target as HTMLInputElement).checked} ?checked=${this.showExonerated}>
        <label for="exonerated" style="color: var(--exonerated-color);">Exonerated</label>
      </div class="filter">
      <div class="filter">
        <input type="checkbox" id="flaky" @change=${(v: MouseEvent) => this.showFlaky = (v.target as HTMLInputElement).checked} ?checked=${this.showFlaky}>
        <label for="flaky" style="color: var(--warning-color);">Flaky</label>
      </div class="filter">
    `;
  }

  static styles = css`
    :host {
      display: inline-block;
    }
    mwc-formfield > mwc-checkbox {
      margin-right: -10px;
    }
    .filter {
      display: inline-block;
      padding: 0 5px;
    }
  `;
}
