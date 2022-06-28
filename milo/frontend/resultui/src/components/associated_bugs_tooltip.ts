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

import '@material/mwc-menu';
import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement } from 'lit-element';
import { html } from 'lit-html';
import { observable } from 'mobx';

import { Cluster, makeRuleLink } from '../services/weetbix';
import commonStyle from '../styles/common_style.css';

@customElement('milo-associated-bugs-tooltip')
export class WeetbixClustersTooltipElement extends MobxLitElement {
  @observable.ref project!: string;
  @observable.ref clusters!: readonly Cluster[];

  protected render() {
    const bugClusters = this.clusters.filter((c) => c.bug);

    return html`
      <table style="padding: 5px;">
        <tbody>
          <tr>
            <td colspan="2">This failure is associated with the following bug(s):</td>
          </tr>
          <tr>
            <td colspan="2"><hr /></td>
          </tr>
          ${bugClusters.map(
            (c) =>
              html`
                <tr class="row">
                  <td><a href=${c.bug!.url}>${c.bug!.linkText}</a></td>
                  <td>
                    <a href=${makeRuleLink(this.project, c.clusterId.id)} target="_blank">Failures</a>
                  </td>
                </tr>
              `
          )}
        </tbody>
      </table>
    `;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: block;
        width: 300px;
      }

      table {
        width: 100%;
      }

      tr > td:first-child {
        width: 100%;
      }
    `,
  ];
}
