/* eslint-disable @typescript-eslint/no-non-null-assertion */
/* eslint-disable @typescript-eslint/indent */
// Copyright 2022 The LUCI Authors.
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
import '@material/mwc-icon';
import '@material/mwc-list/mwc-list-item';
import '@material/mwc-select';

import {
  css,
  customElement,
  html,
  LitElement,
  property,
  state,
  TemplateResult,
} from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { DateTime } from 'luxon';

import {
  DistinctClusterFailure,
  getClustersService,
  QueryClusterFailuresResponse,
} from '../services/cluster';
import {
  countAndSortFailures,
  defaultFailureFilter,
  defaultImpactFilter,
  FailureFilter,
  FailureFilters,
  FailureGroup,
  VariantGroup,
  groupAndCountFailures,
  ImpactFilter,
  ImpactFilters,
  MetricName,
  sortFailureGroups,
  countDistictVariantValues,
} from '../tools/failures_tools';
import {
  clLink,
  clName,
  failureLink,
} from '../tools/urlHandling/links';

// Indent of each level of grouping in the table in pixels.
const levelIndent = 10;

// FailureTable lists the failures in a cluster tracked by Weetbix.
@customElement('failure-table')
export class FailureTable extends LitElement {
  @property()
    project = '';

  @property()
    clusterAlgorithm = '';

  @property()
    clusterID = '';

  @state()
    failures: DistinctClusterFailure[] | undefined;

  @state()
    groups: FailureGroup[] = [];

  @state()
    variants: VariantGroup[] = [];

  @state()
    failureFilter: FailureFilter = defaultFailureFilter;

  @state()
    impactFilter: ImpactFilter = defaultImpactFilter;

  @property()
    sortMetric: MetricName = 'latestFailureTime';

  @property({ type: Boolean })
    ascending = false;

  connectedCallback() {
    super.connectedCallback();

    const service = getClustersService();
    service.queryClusterFailures({
      parent: `projects/${this.project}/clusters/${this.clusterAlgorithm}/${this.clusterID}/failures`,
    }).then((response: QueryClusterFailuresResponse) => {
      this.failures = response.failures;
      this.variants = countDistictVariantValues(response.failures || []);
      this.groupCountAndSortFailures();
    });
  }

  groupCountAndSortFailures() {
    if (this.failures) {
      this.groups = groupAndCountFailures(this.failures, this.variants, this.failureFilter);
    }
    this.groups = countAndSortFailures(this.groups, this.impactFilter);
    this.sortFailures();
  }

  sortFailures() {
    this.groups = sortFailureGroups(this.groups, this.sortMetric, this.ascending);
    this.requestUpdate();
  }

  toggleSort(metric: MetricName) {
    if (metric === this.sortMetric) {
      this.ascending = !this.ascending;
    } else {
      this.sortMetric = metric;
      this.ascending = false;
    }
    this.sortFailures();
  }

  onImpactFilterChanged() {
    const item = this.shadowRoot!.querySelector('#impact-filter [selected]');
    if (item) {
      const selected = item.getAttribute('value');
      this.impactFilter = ImpactFilters.filter((filter) => filter.name == selected)?.[0] || ImpactFilters[1];
    }
    this.groups = countAndSortFailures(this.groups, this.impactFilter);
  }

  onFailureFilterChanged() {
    const item = this.shadowRoot!.querySelector('#failure-filter [selected]');
    if (item) {
      this.failureFilter = (item.getAttribute('value') as FailureFilter) || FailureFilters[0];
    }
    this.groupCountAndSortFailures();
  }

  toggleVariant(variant: VariantGroup) {
    const index = this.variants.indexOf(variant);
    this.variants.splice(index, 1);
    variant.isSelected = !variant.isSelected;
    const numSelected = this.variants.filter((v) => v.isSelected).length;
    this.variants.splice(numSelected, 0, variant);
    this.groupCountAndSortFailures();
  }

  toggleExpand(group: FailureGroup) {
    group.isExpanded = !group.isExpanded;
    this.requestUpdate();
  }

  render() {
    const unselectedVariants = this.variants.filter((v) => !v.isSelected).map((v) => v.key);
    if (this.failures === undefined) {
      return html`Loading cluster failures...`;
    }
    const ungroupedVariants = (failure: DistinctClusterFailure) => {
      return unselectedVariants.map((key) => failure.variant?.def[key] !== undefined ? { key: key, value: failure.variant.def[key] } : null).filter((v) => v);
    };
    const indentStyle = (level: number) => {
      return styleMap({ paddingLeft: (levelIndent * level) + 'px' });
    };
    const groupRow = (group: FailureGroup): TemplateResult => {
      return html`
            <tr>
                ${group.failure ?
        html`<td style=${indentStyle(group.level)}>
                        <a href=${failureLink(group.failure)} target="_blank">${group.failure.ingestedInvocationId}</a>
                        ${(group.failure.changelists !== undefined && group.failure.changelists.length > 0) ?
                            html`(<a href=${clLink(group.failure.changelists[0])}>${clName(group.failure.changelists[0])}</a>)` : html``}
                        <span class="variant-info">${ungroupedVariants(group.failure).map((v) => v && `${v.key}: ${v.value}`).filter((v) => v).join(', ')}</span>
                    </td>` :
        html`<td class="group" style=${indentStyle(group.level)} @click=${() => this.toggleExpand(group)}>
                        <mwc-icon>${group.isExpanded ? 'keyboard_arrow_down' : 'keyboard_arrow_right'}</mwc-icon>
                        ${group.key.value || 'none'} ${group.key.type == 'test' ? html`- <a href="https://ci.chromium.org/ui/test/${this.project}/${group.key.value}" target="_blank">history</a>` : null}
                    </td>`}
                <td class="number">
                    ${group.failure ?
        (group.failure.presubmitRun ?
            html`<a class="presubmit-link" href="https://luci-change-verifier.appspot.com/ui/run/${group.failure.presubmitRun.presubmitRunId.id}" target="_blank">${group.presubmitRejects}</a>` :
            '-') : group.presubmitRejects}
                </td>
                <td class="number">${group.invocationFailures}</td>
                <td class="number">${group.criticalFailuresExonerated}</td>
                <td class="number">${group.failures}</td>
                <td>${DateTime.fromISO(group.latestFailureTime).toRelative()}</td>
            </tr>
            ${group.isExpanded ? group.children.map((child) => groupRow(child)) : null}`;
    };
    const groupByButton = (variant: VariantGroup) => {
      return html`
                <mwc-button
                    label=${`${variant.key} (${variant.values.length})`}
                    ?unelevated=${variant.isSelected}
                    ?outlined=${!variant.isSelected}
                    @click=${() => this.toggleVariant(variant)}></mwc-button>`;
    };
    return html`
            <div class="controls">
                <div class="select-offset">
                    <mwc-select id="failure-filter" outlined label="Failure Type" @change=${() => this.onFailureFilterChanged()}>
                        ${FailureFilters.map((filter) => html`<mwc-list-item ?selected=${filter == this.failureFilter} value="${filter}">${filter}</mwc-list-item>`)}
                    </mwc-select>
                </div>
                <div class="select-offset">
                    <mwc-select id="impact-filter" outlined label="Impact" @change=${() => this.onImpactFilterChanged()}>
                        ${ImpactFilters.map((filter) => html`<mwc-list-item ?selected=${filter == this.impactFilter} value="${filter.name}">${filter.name}</mwc-list-item>`)}
                    </mwc-select>
                </div>
                <div>
                    <div class="label">
                        Group By
                    </div>
                    ${this.variants.map((v) => groupByButton(v))}
                </div>
            </div>
            <table data-testid="failures-table">
                <thead>
                    <tr>
                        <th></th>
                        <th class="sortable" @click=${() => this.toggleSort('presubmitRejects')}>
                            User Cls Failed Presubmit
                            ${this.sortMetric === 'presubmitRejects' ? html`<mwc-icon>${this.ascending ? 'expand_less' : 'expand_more'}</mwc-icon>` : null}
                        </th>
                        <th class="sortable" @click=${() => this.toggleSort('invocationFailures')}>
                            Builds Failed
                            ${this.sortMetric === 'invocationFailures' ? html`<mwc-icon>${this.ascending ? 'expand_less' : 'expand_more'}</mwc-icon>` : null}
                        </th>
                        <th class="sortable" @click=${() => this.toggleSort('criticalFailuresExonerated')}>
                            Presubmit-Blocking Failures Exonerated
                            ${this.sortMetric === 'criticalFailuresExonerated' ? html`<mwc-icon>${this.ascending ? 'expand_less' : 'expand_more'}</mwc-icon>` : null}
                        </th>
                        <th class="sortable" @click=${() => this.toggleSort('failures')}>
                            Total Failures
                            ${this.sortMetric === 'failures' ? html`<mwc-icon>${this.ascending ? 'expand_less' : 'expand_more'}</mwc-icon>` : null}
                        </th>
                        <th class="sortable" @click=${() => this.toggleSort('latestFailureTime')}>
                            Latest Failure Time
                            ${this.sortMetric === 'latestFailureTime' ? html`<mwc-icon>${this.ascending ? 'expand_less' : 'expand_more'}</mwc-icon>` : null}
                        </th>
                    </tr>
                </thead>
                <tbody>
                    ${this.groups.map((group) => groupRow(group))}
                </tbody>
            </table>
        `;
  }
  static styles = [css`
        .controls {
            display: flex;
            gap: 30px;
        }
        .label {
            color: var(--greyed-out-text-color);
            font-size: var(--font-size-small);
        }
        .select-offset {
            padding-top: 7px
        }
        #impact-filter {
            width: 280px;
        }
        table {
            border-collapse: collapse;
            width: 100%;
            table-layout: fixed;
        }
        th {
            font-weight: normal;
            color: var(--greyed-out-text-color);
            font-size: var(--font-size-small);
            text-align: left;
        }
        td,th {
            padding: 4px;
            max-width: 80%;
        }
        td.number {
            text-align: right;
        }
        td.group {
            word-break: break-all;
        }
        th.sortable {
            cursor: pointer;
            width:120px;
        }
        tbody tr:hover {
            background-color: var(--light-active-color);
        }
        .group {
            cursor: pointer;
            --mdc-icon-size: var(--font-size-default);
        }
        .variant-info {
            color: var(--greyed-out-text-color);
            font-size: var(--font-size-small);
        }
        .presubmit-link {
            font-size: var(--font-size-small);
        }
 `];
}
