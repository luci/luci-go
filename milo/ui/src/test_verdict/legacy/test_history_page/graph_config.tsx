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
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { styleMap } from 'lit/directives/style-map.js';
import { computed, makeObservable, observable } from 'mobx';

import { consumeStore, StoreInstance } from '@/common/store';
import { GraphType, XAxisType } from '@/common/store/test_history_page';
import { commonStyles } from '@/common/styles/stylesheets';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { consumer } from '@/generic_libs/tools/lit_context';

@customElement('milo-th-graph-config')
@consumer
export class TestHistoryGraphConfigElement extends MobxExtLitElement {
  @observable.ref @consumeStore() store!: StoreInstance;
  @computed get pageState() {
    return this.store.testHistoryPage;
  }

  @computed private get shouldShowCountFilter() {
    return (
      this.pageState.graphType === GraphType.STATUS &&
      this.pageState.xAxisType === XAxisType.DATE
    );
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    return html`
      <div>
        <label>Show:</label>
        <select
          @input=${(e: InputEvent) => {
            this.pageState.setGraphType(
              (e.target as HTMLOptionElement).value as GraphType,
            );
          }}
        >
          <option
            ?selected=${this.pageState.graphType === GraphType.STATUS}
            value=${GraphType.STATUS}
          >
            Status
          </option>
          <option
            ?selected=${this.pageState.graphType === GraphType.DURATION}
            value=${GraphType.DURATION}
          >
            Duration
          </option>
        </select>
      </div>
      <div
        id="duration-filter"
        class="filter"
        style=${styleMap({
          display:
            this.pageState.graphType === GraphType.DURATION ? '' : 'none',
        })}
      >
        <input id="pass-only-toggle" type="checkbox" checked disabled />
        <label for="pass-only-toggle"
          >Pass Only<mwc-icon
            class="inline-icon"
            title="Only include durations from passed results"
            >info</mwc-icon
          ></label
        >
      </div>
      <div>
        <label>X-Axis:</label>
        <select
          disabled
          @input=${(e: InputEvent) => {
            this.pageState.setXAxisType(
              (e.target as HTMLOptionElement).value as XAxisType,
            );
          }}
        >
          <option
            ?selected=${this.pageState.xAxisType === XAxisType.COMMIT}
            value=${XAxisType.COMMIT}
          >
            Commit
          </option>
          <option
            ?selected=${this.pageState.xAxisType === XAxisType.DATE}
            value=${XAxisType.DATE}
          >
            Date
          </option>
        </select>
      </div>
      <div
        id="count-filter"
        style=${styleMap({ display: this.shouldShowCountFilter ? '' : 'none' })}
      >
        <label>Count:</label>
        <div class="filter">
          <input
            id="unexpected-toggle"
            type="checkbox"
            ?checked=${this.pageState.countUnexpected}
            @change=${(e: Event) =>
              this.pageState.setCountUnexpected(
                (e.target as HTMLInputElement).checked,
              )}
          />
          <label for="unexpected-toggle" style="color: var(--failure-color);"
            >Unexpected</label
          >
        </div>
        <div class="filter">
          <input
            id="unexpectedly-skipped-toggle"
            type="checkbox"
            ?checked=${this.pageState.countUnexpectedlySkipped}
            @change=${(e: Event) =>
              this.pageState.setCountUnexpectedlySkipped(
                (e.target as HTMLInputElement).checked,
              )}
          />
          <label
            for="unexpectedly-skipped-toggle"
            style="color: var(--critical-failure-color);"
            >Unexpectedly Skipped</label
          >
        </div>
        <div class="filter">
          <input
            id="flaky-toggle"
            type="checkbox"
            ?checked=${this.pageState.countFlaky}
            @change=${(e: Event) =>
              this.pageState.setCountFlaky(
                (e.target as HTMLInputElement).checked,
              )}
          />
          <label for="flaky-toggle" style="color: var(--warning-color);"
            >Flaky</label
          >
        </div>
        <div class="filter">
          <input
            id="exonerated-toggle"
            type="checkbox"
            ?checked=${this.pageState.countExonerated}
            @change=${(e: Event) =>
              this.pageState.setCountExonerated(
                (e.target as HTMLInputElement).checked,
              )}
          />
          <label for="exonerated-toggle" style="color: var(--exonerated-color);"
            >Exonerated</label
          >
        </div>
      </div>
    `;
  }

  static styles = [
    commonStyles,
    css`
      :host {
        display: grid;
        height: 32px;
        padding: 5px;
        grid-template-columns: auto auto 1fr;
        grid-gap: 15px;
      }

      select {
        display: inline-block;
        box-sizing: border-box;
        padding: 0.3rem 0.5rem;
        font-size: 1rem;
        background-clip: padding-box;
        border: 1px solid var(--divider-color);
        border-radius: 0.25rem;
        transition:
          border-color 0.15s ease-in-out,
          box-shadow 0.15s ease-in-out;
        text-overflow: ellipsis;
      }

      #duration-filter {
        height: 100%;
        line-height: 32px;
      }

      #count-filter {
        display: grid;
        grid-template-columns: auto auto auto auto 1fr;
        grid-gap: 8px;
        height: 100%;
        line-height: 32px;
      }
      .filter {
        display: inline-block;
      }
      .filter > input {
        margin-right: -1px;
      }

      .inline-icon {
        --mdc-icon-size: 1.2em;
        vertical-align: middle;
      }
    `,
  ];
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-th-graph-config': Record<string, never>;
    }
  }
}

export function GraphConfig() {
  return <milo-th-graph-config />;
}
