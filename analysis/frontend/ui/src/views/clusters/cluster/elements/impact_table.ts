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

import {
  customElement,
  html,
  LitElement,
  property,
} from 'lit-element';
import { Ref } from 'react';

import { Cluster, Counts } from '../../../../services/cluster';

const metric = (counts: Counts): string => {
  return counts.nominal || '0';
};

@customElement('impact-table')
export class ImpactTable extends LitElement {
  @property({ attribute: false })
    currentCluster!: Cluster;

  @property({ attribute: false })
    ref: Ref<ImpactTable> | null = null;

  render() {
    return html`
    <table data-testid="impact-table">
        <thead>
            <tr>
                <th></th>
                <th>1 day</th>
                <th>3 days</th>
                <th>7 days</th>
            </tr>
        </thead>
        <tbody class="data">
            <tr>
                <th>User Cls Failed Presubmit</th>
                <td class="number">${metric(this.currentCluster.userClsFailedPresubmit.oneDay)}</td>
                <td class="number">${metric(this.currentCluster.userClsFailedPresubmit.threeDay)}</td>
                <td class="number">${metric(this.currentCluster.userClsFailedPresubmit.sevenDay)}</td>
            </tr>
            <tr>
                <th>Presubmit-Blocking Failures Exonerated</th>
                <td class="number">${metric(this.currentCluster.criticalFailuresExonerated.oneDay)}</td>
                <td class="number">${metric(this.currentCluster.criticalFailuresExonerated.threeDay)}</td>
                <td class="number">${metric(this.currentCluster.criticalFailuresExonerated.sevenDay)}</td>
            </tr>
            <tr>
                <th>Total Failures</th>
                <td class="number">${metric(this.currentCluster.failures.oneDay)}</td>
                <td class="number">${metric(this.currentCluster.failures.threeDay)}</td>
                <td class="number">${metric(this.currentCluster.failures.sevenDay)}</td>
            </tr>
        </tbody>
    </table>`;
  }
}
