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
      <div id="details-header" @click=${() => this.expanded = !this.expanded}>
        <mwc-icon id="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <span id="details-header-text">Details</span>
      </div>
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
    `;
  }

  static styles = css`
    #details-header {
      display: flex;
      padding: 6px 16px;
      font-size: 14px;
      line-height: 16px;
      font-family: "Google Sans", "Helvetica Neue", sans-serif;
      cursor: pointer;
      letter-spacing: 0.15px;
    }

    #expand-toggle {
      font-size: 16px;
    }

    #details-header-text {
      margin-left: 4px;
    }

    #content {
      padding: 5px 24px;
      border-top: 1px solid rgb(238,238,238);
    }

    #included-invocations ul {
      list-style-type: none;
      margin-block-start: auto;
      margin-block-end: auto;
      padding-inline-start: 32px;
    }

    #tag-table {
      margin-left: 29px;
    }
  `;
}
