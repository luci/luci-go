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
import { computed, observable } from 'mobx';

import { consumeConfigsStore, UserConfigsStore } from '../context/app_state/user_configs';
import { consumeInvocationState, InvocationState } from '../context/invocation_state/invocation_state';

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
@consumeConfigsStore
@consumeInvocationState
export class TestFilterElement extends MobxLitElement {
  @observable.ref configsStore!: UserConfigsStore;
  @observable.ref invocationState!: InvocationState;

  @computed private get testFilters() { return this.configsStore.userConfigs.tests; }
  @computed private get unexpectedVariantCount() {
    return this.invocationState.testLoader?.unexpectedVariantCount || '0+';
  }
  @computed private get flakyVariantCount() {
    return this.invocationState.testLoader?.flakyVariantCount || '0+';
  }
  @computed private get exoneratedVariantCount() {
    return this.invocationState.testLoader?.exoneratedVariantCount || '0+';
  }
  @computed private get expectedVariantCount() {
    return this.invocationState.testLoader?.expectedVariantCount || '0+';
  }


  protected render() {
    return html`
      Show:
      <div class="filter">
        <input
          type="checkbox"
          id="unexpected"
          @change=${(v: MouseEvent) => {
            this.testFilters.showUnexpectedVariant = (v.target as HTMLInputElement).checked;
            this.configsStore.save();
          }}
          .checked=${this.testFilters.showUnexpectedVariant}
        >
        <label for="unexpected" style="color: var(--failure-color);">Unexpected (${this.unexpectedVariantCount})</label>
      </div class="filter">
      <div class="filter">
        <input
          type="checkbox"
          id="flaky"
          @change=${(v: MouseEvent) => {
            this.testFilters.showFlakyVariant = (v.target as HTMLInputElement).checked;
            this.configsStore.save();
          }}
          .checked=${this.testFilters.showFlakyVariant}
        >
        <label for="flaky" style="color: var(--warning-color);">Flaky (${this.flakyVariantCount})</label>
      </div class="filter">
      <div class="filter">
        <input
          type="checkbox"
          id="exonerated"
          @change=${(v: MouseEvent) => {
            this.testFilters.showExoneratedVariant = (v.target as HTMLInputElement).checked;
            this.configsStore.save();
          }}
          .checked=${this.testFilters.showExoneratedVariant}
        >
        <label for="exonerated" style="color: var(--exonerated-color);">Exonerated (${this.exoneratedVariantCount})</label>
      </div class="filter">
      <div class="filter">
        <input
          type="checkbox"
          id="expected"
          @change=${(v: MouseEvent) => {
            this.testFilters.showExpectedVariant = (v.target as HTMLInputElement).checked;
            this.configsStore.save();
          }}
          .checked=${this.testFilters.showExpectedVariant}
        >
      <label for="expected" style="color: var(--success-color);">Expected (${this.expectedVariantCount})</label>
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
