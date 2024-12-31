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
import { Interpolation, Theme } from '@emotion/react';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { styleMap } from 'lit/directives/style-map.js';
import { computed, makeObservable, observable } from 'mobx';

import '@/common/components/auto_complete';
import '@/generic_libs/components/hotkey';
import { SuggestionEntry } from '@/common/components/auto_complete';
import { suggestTestHistoryFilterQuery } from '@/common/queries/th_filter_query';
import { consumeStore, StoreInstance } from '@/common/store';
import { consumer } from '@/generic_libs/tools/lit_context';

/**
 * An element that let the user search tests in the test history view with DSL.
 */
@customElement('milo-th-filter-box')
@consumer
export class TestHistoryFilterBoxElement extends MobxLitElement {
  static get properties() {
    return {
      initialFilterText: { type: String },
    };
  }

  private _initialFilterText = '';
  set initialFilterText(newVal: string) {
    this._initialFilterText = newVal;
  }
  get initialFilterText() {
    return this._initialFilterText;
  }

  @observable.ref @consumeStore() store!: StoreInstance;
  @computed get pageState() {
    return this.store.testHistoryPage;
  }

  @observable.ref private uncommittedFilterText = '';

  @computed private get lastSubQuery() {
    return this.uncommittedFilterText.split(' ').pop() || '';
  }
  @computed private get queryPrefix() {
    const searchTextPrefixLen =
      this.uncommittedFilterText.length - this.lastSubQuery.length;
    return this.uncommittedFilterText.slice(0, searchTextPrefixLen);
  }
  @computed private get suggestions() {
    return suggestTestHistoryFilterQuery(this.uncommittedFilterText);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  commitFilter() {
    this.pageState.setFilterText(this.uncommittedFilterText);
  }

  connectedCallback() {
    super.connectedCallback();
    this.uncommittedFilterText = this.initialFilterText;
  }

  protected render() {
    return html`
      <milo-hotkey
        .key=${'/'}
        .handler=${() => {
          // Set a tiny timeout to ensure '/' isn't recorded by the input box.
          setTimeout(() =>
            this.shadowRoot?.getElementById('input-box')!.focus(),
          );
        }}
      >
        <milo-auto-complete
          id="input-box"
          .highlight=${true}
          .value=${this.uncommittedFilterText}
          .placeHolder=${'Press / to filter test history...'}
          .suggestions=${this.suggestions}
          .onValueUpdate=${(newVal: string) =>
            (this.uncommittedFilterText = newVal)}
          .onSuggestionSelected=${(suggestion: SuggestionEntry) => {
            this.uncommittedFilterText =
              this.queryPrefix + suggestion.value! + ' ';
            this.commitFilter();
          }}
          .onComplete=${() => this.commitFilter()}
        >
          <mwc-icon
            style=${styleMap({
              color:
                this.uncommittedFilterText === '' ? '' : 'var(--active-color)',
            })}
            slot="pre-icon"
          >
            filter_alt
          </mwc-icon>
          ${this.pageState.filterText === this.uncommittedFilterText
            ? html`
                <mwc-icon
                  id="clear-filter"
                  slot="post-icon"
                  title="Clear"
                  style=${styleMap({
                    display: this.uncommittedFilterText === '' ? 'none' : '',
                  })}
                  @click=${() => {
                    this.uncommittedFilterText = '';
                    this.commitFilter();
                  }}
                >
                  close
                </mwc-icon>
              `
            : html`
                <div
                  id="commit-filter"
                  slot="post-icon"
                  title="Press Enter to apply the filter"
                  @click=${() => this.commitFilter()}
                >
                  Apply
                  <mwc-icon>keyboard_return</mwc-icon>
                </div>
              `}
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

    #clear-filter {
      color: var(--delete-color);
      cursor: pointer;
    }

    #commit-filter {
      color: var(--active-color);
      cursor: pointer;
    }
    #commit-filter mwc-icon {
      vertical-align: middle;
    }
  `;
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-th-filter-box': {
        initialFilterText: string;
        css?: Interpolation<Theme>;
        class?: string;
      };
    }
  }
}

export interface FilterBoxProps {
  readonly initialFilterText: string;
  readonly css?: Interpolation<Theme>;
  readonly className?: string;
}

export function FilterBox(props: FilterBoxProps) {
  return <milo-th-filter-box {...props} class={props.className} />;
}
