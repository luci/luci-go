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

import '@material/mwc-icon';
import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, makeObservable, observable } from 'mobx';

import '../../components/auto_complete';
import '../../components/hotkey';
import { SuggestionEntry } from '../../components/auto_complete';
import { consumeInvocationState, InvocationState } from '../../context/invocation_state';
import { consumer } from '../../libs/context';
import { suggestTestResultSearchQuery } from '../../libs/queries/tr_search_query';

/**
 * An element that let the user search tests in the test results tab with DSL.
 */
@customElement('milo-trt-search-box')
@consumer
export class TestResultTabSearchBoxElement extends MobxLitElement {
  @observable.ref
  @consumeInvocationState()
  invocationState!: InvocationState;

  @computed private get lastSubQuery() {
    return this.invocationState.searchText.split(' ').pop() || '';
  }
  @computed private get queryPrefix() {
    const searchTextPrefixLen = this.invocationState.searchText.length - this.lastSubQuery.length;
    return this.invocationState.searchText.slice(0, searchTextPrefixLen);
  }
  @computed private get suggestions() {
    return suggestTestResultSearchQuery(this.invocationState.searchText);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    return html`
      <milo-hotkey
        .key="/"
        .handler=${() => {
          // Set a tiny timeout to ensure '/' isn't recorded by the input box.
          setTimeout(() => this.shadowRoot?.getElementById('search-box')!.focus());
        }}
      >
        <milo-auto-complete
          id="search-box"
          .highlight=${true}
          .value=${this.invocationState.searchText}
          .placeHolder=${'Press / to search test results...'}
          .suggestions=${this.suggestions}
          .onValueUpdate=${(newVal: string) => (this.invocationState.searchText = newVal)}
          .onSuggestionSelected=${(suggestion: SuggestionEntry) => {
            this.invocationState.searchText = this.queryPrefix + suggestion.value! + ' ';
          }}
        >
          <mwc-icon
            style=${styleMap({ color: this.invocationState.searchText === '' ? '' : 'var(--active-color)' })}
            slot="pre-icon"
          >
            search
          </mwc-icon>
          <mwc-icon
            id="clear-search"
            slot="post-icon"
            title="Clear"
            style=${styleMap({ display: this.invocationState.searchText === '' ? 'none' : '' })}
            @click=${() => (this.invocationState.searchText = '')}
          >
            close
          </mwc-icon>
        </milo-auto-complete>
      </milo-hotkey>
    `;
  }

  static styles = css`
    :host {
      display: inline-block;
    }

    @keyframes highlight {
      from {
        background-color: var(--highlight-background-color);
      }
      to {
        background-color: inherit;
      }
    }

    mwc-icon {
      margin: 2px;
    }

    #clear-search {
      color: var(--delete-color);
      cursor: pointer;
    }
  `;
}
