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
import '@material/mwc-icon';
import { css, customElement } from 'lit-element';
import { html } from 'lit-html';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable } from 'mobx';

import { Invocation } from '../services/resultdb';


@customElement('tr-invocation-details')
export class InvocationDetailsElement extends MobxLitElement {
  // Must be provided by the parent element.
  @observable.ref
  invocation?: Invocation;

  @observable.ref
  expanded = false;

  @computed
  private get hasIncludedInvocations() {
    return this.invocation!.includedInvocations.length > 0;
  }
  @computed
  private get hasTags() {
    return this.invocation!.tags.length > 0;
  }

  protected render() {
    return html`
      <div
        class="expandable-header"
        @click=${() => this.expanded = !this.expanded}
      >
        <mwc-icon class="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <span class="one-line-content">Details</span>
      </div>
      <div id="body">
        <div id="content" style=${styleMap({'display': this.expanded ? '' : 'none'})}>
          <div>Create Time: ${new Date(this.invocation!.createTime).toLocaleString()}</div>
          <div>Finalize Time: ${new Date(this.invocation!.finalizeTime).toLocaleDateString()}</div>
          <div>Deadline: ${new Date(this.invocation!.deadline).toLocaleDateString()}</div>
          <div
            id="included-invocations"
            style=${styleMap({'display': this.hasIncludedInvocations ? '' : 'none'})}
          >Included Invocations:
            <ul>
            ${this.invocation!.includedInvocations.map(invocationName => html`
              <li><a href="/invocation/${encodeURIComponent(invocationName)}">${invocationName.slice('invocations/'.length)}</a></li>
            `)}
            </ul>
          </div>
          <div style=${styleMap({'display': this.hasTags ? '' : 'none'})}>Tags:
            <table id="tag-table" border="0">
            ${this.invocation!.tags.map((tag) => html`
              <tr>
                <td>${tag.key}:</td>
                <td>${tag.value}</td>
              </tr>
            `)}
            </table>
          </div>
        </div>
      </div>
    `;
  }

  static styles = css`
    .expandable-header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: 24px;
      grid-gap: 5px;
      cursor: pointer;
      letter-spacing: 0.15px;
    }
    .expandable-header .expand-toggle {
      grid-row: 1;
      grid-column: 1;
    }
    .expandable-header .one-line-content {
      grid-row: 1;
      grid-column: 2;
      font-size: 16px;
      line-height: 24px;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }
    #body {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-gap: 5px;
    }
    #content {
      padding: 5px 0 5px 0;
      grid-column: 2;
    }
    #content-ruler {
      border-left: 1px solid grey;
      width: 0px;
      margin-left: 11.5px;
    }

    #included-invocation {
      list-style-type: none;
    }

    #tag-table {
      margin-left: 29px;
    }
  `;
}
