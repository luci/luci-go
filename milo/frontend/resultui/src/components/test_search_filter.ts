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
import { css, customElement, html } from 'lit-element';
import { observable } from 'mobx';

import { consumeInvocationState, InvocationState } from '../context/invocation_state/invocation_state';

export interface TestFilter {
  showExpected: boolean;
  showExonerated: boolean;
  showFlaky: boolean;
}

/**
 * An element that let the user search tests with DSL.
 */
@customElement('milo-test-search-filter')
@consumeInvocationState
export class TestSearchFilterElement extends MobxLitElement {
  @observable.ref invocationState!: InvocationState;

  protected render() {
    return html`
      <milo-hotkey
        key="/"
        .handler=${() => {
          // Set a tiny timeout to ensure '/' isn't recorded by the input box.
          setTimeout(() => this.shadowRoot?.getElementById('search-box')?.focus());
        }}
      >
        <input
          id="search-box"
          placeholder="Press / to search test results..."
          .value=${this.invocationState.searchText}
          @input=${(e: InputEvent) => this.invocationState.searchText = (e.target as HTMLInputElement).value}
        >
      </milo-hotkey>
    `;
  }

  static styles = css`
    :host {
      display: inline-block;
    }

    #search-box {
      margin-left: 5px;
      display: inline-block;
      width: 350px;
      padding: .3rem .5rem;
      font-size: 1rem;
      color: var(--light-text-color);
      background-clip: padding-box;
      border: 1px solid var(--divider-color);
      border-radius: .25rem;
      transition: border-color .15s ease-in-out,box-shadow .15s ease-in-out;
      text-overflow: ellipsis;
    }
  `;
}
