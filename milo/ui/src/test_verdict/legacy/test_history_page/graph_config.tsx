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
      (this.pageState.graphType === GraphType.STATUS_WITH_EXONERATION ||
        this.pageState.graphType === GraphType.STATUS_WITHOUT_EXONERATION) &&
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
            ?selected=${this.pageState.graphType ===
            GraphType.STATUS_WITH_EXONERATION}
            value=${GraphType.STATUS_WITH_EXONERATION}
          >
            Status after Exoneration
          </option>
          <option
            ?selected=${this.pageState.graphType ===
            GraphType.STATUS_WITHOUT_EXONERATION}
            value=${GraphType.STATUS_WITHOUT_EXONERATION}
          >
            Status before Exoneration
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
            id="passed-toggle"
            type="checkbox"
            ?checked=${this.pageState.countPassed}
            @change=${(e: Event) =>
              this.pageState.setCountPassed(
                (e.target as HTMLInputElement).checked,
              )}
          />
          <label
            for="passed-toggle"
            style="color: var(--success-color);"
            title="The test obtained a passing result."
            >Passed</label
          >
        </div>
        <div class="filter">
          <input
            id="failed-toggle"
            type="checkbox"
            ?checked=${this.pageState.countFailed}
            @change=${(e: Event) =>
              this.pageState.setCountFailed(
                (e.target as HTMLInputElement).checked,
              )}
          />
          <label
            for="failed-toggle"
            style="color: var(--failure-color);"
            title="The test obtained a failing result."
            >Failed</label
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
          <label
            for="flaky-toggle"
            style="color: var(--warning-color);"
            title="The test had both passing and failing results on the same code under test."
            >Flaky</label
          >
        </div>
        <div class="filter">
          <input
            id="skipped-toggle"
            type="checkbox"
            ?checked=${this.pageState.countSkipped}
            @change=${(e: Event) =>
              this.pageState.setCountSkipped(
                (e.target as HTMLInputElement).checked,
              )}
          />
          <label
            for="skipped-toggle"
            style="color: var(--skipped-color);"
            title="The test was intentionally not run. For example, because it is disabled in code."
            >Skipped</label
          >
        </div>
        <div class="filter">
          <input
            id="execution-errored-toggle"
            type="checkbox"
            ?checked=${this.pageState.countExecutionErrored}
            @change=${(e: Event) =>
              this.pageState.setCountExecutionErrored(
                (e.target as HTMLInputElement).checked,
              )}
          />
          <label
            for="execution-errored-toggle"
            style="color: var(--critical-failure-color);"
            title="There was a problem running the test."
            >Execution Errored</label
          >
        </div>
        <div class="filter">
          <input
            id="precluded-toggle"
            type="checkbox"
            ?checked=${this.pageState.countPrecluded}
            @change=${(e: Event) =>
              this.pageState.setCountPrecluded(
                (e.target as HTMLInputElement).checked,
              )}
          />
          <label
            for="precluded-toggle"
            style="color: var(--precluded-color);"
            title="The test was not run because of a problem at a higher-level (e.g. device provisioning issue or suite-level timeout hit)."
            >Precluded</label
          >
        </div>
        <div
          class="filter"
          style=${styleMap({
            display:
              this.pageState.graphType === GraphType.STATUS_WITH_EXONERATION
                ? ''
                : 'none',
          })}
        >
          <input
            id="exonerated-toggle"
            type="checkbox"
            ?checked=${this.pageState.countExonerated}
            @change=${(e: Event) =>
              this.pageState.setCountExonerated(
                (e.target as HTMLInputElement).checked,
              )}
          />
          <label
            for="exonerated-toggle"
            style="color: var(--exonerated-color);"
            title="The test failed (or execution errored or was precluded), but the CL under test was absolved from blame."
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
        grid-template-columns: auto auto auto auto auto auto auto 1fr;
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

declare module 'react' {
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
