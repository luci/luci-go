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

import '@material/mwc-button';
import '@material/mwc-dialog';
import '@material/mwc-icon';
import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement, html } from 'lit-element';
import { observable } from 'mobx';

import { consumer } from '../../libs/context';
import commonStyle from '../../styles/common_style.css';
import { consumeTestVariantTableState, TestVariantTableState } from './context';

@customElement('milo-tvt-config-widget')
@consumer
export class TestVariantsTableConfigWidgetElement extends MobxLitElement {
  @observable.ref
  @consumeTestVariantTableState()
  tableState!: TestVariantTableState;

  // These properties are frequently updated.
  // Don't set them as observables so updating them won't have big performance
  // impact.
  private uncommittedColumnKeys: readonly string[] = [];
  private uncommittedSortingKeys: readonly string[] = [];
  private uncommittedGroupingKeys: readonly string[] = [];

  @observable.ref private showTableConfigDialog = false;

  private renderPropKeysConfigRow(label: string, keys: readonly string[], updateKeys: (newKeys: string[]) => void) {
    return html`
      <tr>
        <td>${label}:</td>
        <td>
          <input
            .value=${keys.join(', ')}
            placeholder="A list of comma-separated property keys (e.g. v.test_suite,v.gpu)."
            @input=${(e: InputEvent) => {
              const newKeys = (e.target as HTMLInputElement).value
                .split(',')
                .map((k) => k.trim())
                .filter((k) => k !== '');
              updateKeys(newKeys);
            }}
          />
        </td>
      </tr>
    `;
  }

  protected render() {
    return html`
      <div
        id="configure-table"
        class="filters-container"
        @click=${() => {
          this.uncommittedColumnKeys = this.tableState.columnKeys;
          this.uncommittedSortingKeys = this.tableState.sortingKeys;
          this.uncommittedGroupingKeys = this.tableState.groupingKeys;
          this.showTableConfigDialog = true;
        }}
      >
        <mwc-icon class="inline-icon">table_chart</mwc-icon>
        <span>Configure Table</span>
      </div>
      <mwc-dialog
        id="table-config-dialog"
        heading="Table Configuration"
        ?open=${this.showTableConfigDialog}
        @closed=${(event: CustomEvent<{ action: string }>) => {
          if (event.detail.action === 'apply') {
            this.tableState.setColumnKeys(this.uncommittedColumnKeys);
            this.tableState.setSortingKeys(this.uncommittedSortingKeys);
            this.tableState.setGroupingKeys(this.uncommittedGroupingKeys);
          }
          this.showTableConfigDialog = false;
        }}
      >
        <table>
          ${this.renderPropKeysConfigRow(
            'Additional columns',
            this.uncommittedColumnKeys,
            (newKeys) => (this.uncommittedColumnKeys = newKeys)
          )}
          ${this.renderPropKeysConfigRow(
            'Sort by',
            this.uncommittedSortingKeys,
            (newKeys) => (this.uncommittedSortingKeys = newKeys)
          )}
          ${
            this.tableState.enablesGrouping
              ? this.renderPropKeysConfigRow(
                  'Group by',
                  this.uncommittedGroupingKeys,
                  (newKeys) => (this.uncommittedGroupingKeys = newKeys)
                )
              : ''
          }
          </tr>
        </table>
        <mwc-button
          id="reset-table-config"
          dense
          unelevated
          @click=${() => {
            this.uncommittedColumnKeys = this.tableState.defaultColumnKeys;
            this.uncommittedSortingKeys = this.tableState.defaultSortingKeys;
            this.uncommittedGroupingKeys = this.tableState.defaultGroupingKeys;

            // this.uncommittedXXXKeys are not observables.
            // Manually trigger an updated.
            this.update(new Map());
          }}
        >
          Reset to default
        </mwc-button>
        <p>A key must be one of the following:</p>
        <ol>
          <li>'status': status of the test variant (e.g. Unexpected, Flaky).</li>
          <li>'name': name of the test variant.</li>
          <li>'v.{variant_key}': variant key of the test variant (e.g. v.gpu).</li>
        </ol>
        <p>Sorting keys can have '-' prefix to sort in descending order (e.g. -status, -v.gpu).</p>
        <!-- TODO(weiweilin): add link to a more detailed instruction. -->
        <mwc-button slot="primaryAction" dialogAction="apply" dense unelevated>Apply</mwc-button>
        <mwc-button slot="secondaryAction" dialogAction="cancel">Cancel</mwc-button>
      </mwc-dialog>
`;
  }

  static styles = [
    commonStyle,
    css`
      #configure-table {
        cursor: pointer;
        line-height: 24px;
        color: var(--active-text-color);
      }

      #table-config-dialog {
        --mdc-dialog-min-width: 700px;
      }
      #table-config-dialog table {
        width: 100%;
      }
      #table-config-dialog table td:first-child {
        width: 160px;
      }
      #table-config-dialog p {
        margin-block-start: 0.5em;
        margin-block-end: 0.5em;
        margin-inline-start: 4px;
      }
      #table-config-dialog ol {
        margin-block-start: 0.5em;
        margin-block-end: 0.5em;
      }

      input {
        display: inline-block;
        width: 100%;
        box-sizing: border-box;
        padding: 0.3rem 0.5rem;
        font-size: 1rem;
        background-clip: padding-box;
        border: 1px solid var(--divider-color);
        border-radius: 0.25rem;
        transition: border-color 0.15s ease-in-out, box-shadow 0.15s ease-in-out;
        text-overflow: ellipsis;
      }

      .inline-icon {
        vertical-align: bottom;
      }
    `,
  ];
}
