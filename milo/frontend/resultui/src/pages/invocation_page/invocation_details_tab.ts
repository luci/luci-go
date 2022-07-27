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

import '@material/mwc-icon';
import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement } from 'lit-element';
import { html } from 'lit-html';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, makeObservable, observable } from 'mobx';

import { AppState, consumeAppState } from '../../context/app_state';
import { consumeInvocationState, InvocationState } from '../../context/invocation_state';
import { consumer } from '../../libs/context';
import { router } from '../../routes';
import commonStyle from '../../styles/common_style.css';

function stripInvocationPrefix(invocationName: string): string {
  return invocationName.slice('invocations/'.length);
}

@customElement('milo-invocation-details-tab')
@consumer
export class InvocationDetailsTabElement extends MobxLitElement {
  @observable.ref
  @consumeAppState()
  appState!: AppState;

  @observable.ref
  @consumeInvocationState()
  invocationState!: InvocationState;

  @computed
  private get hasIncludedInvocations() {
    return (this.invocationState.invocation!.includedInvocations || []).length > 0;
  }
  @computed
  private get hasTags() {
    return (this.invocationState.invocation!.tags || []).length > 0;
  }

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'invocation-details';
  }

  protected render() {
    const invocation = this.invocationState.invocation;
    if (invocation === null) {
      return html``;
    }
    return html`
      <div>Create Time: ${new Date(invocation.createTime).toLocaleString()}</div>
      <div>Finalize Time: ${new Date(invocation.finalizeTime).toLocaleDateString()}</div>
      <div>Deadline: ${new Date(invocation.deadline).toLocaleDateString()}</div>
      <div id="included-invocations" style=${styleMap({ display: this.hasIncludedInvocations ? '' : 'none' })}>
        Included Invocations:
        <ul>
          ${invocation.includedInvocations
            ?.map((invName) => stripInvocationPrefix(invName))
            .map(
              (invId) => html`
                <li>
                  <a href=${router.urlForName('invocation', { invocation_id: invId })} target="_blank">${invId}</a>
                  ${this.renderBuildLink(invId)}
                </li>
              `
            )}
        </ul>
      </div>
      <div style=${styleMap({ display: this.hasTags ? '' : 'none' })}>
        Tags:
        <table id="tag-table" border="0">
          ${invocation.tags?.map(
            (tag) => html`
              <tr>
                <td>${tag.key}:</td>
                <td>${tag.value}</td>
              </tr>
            `
          )}
        </table>
      </div>
    `;
  }

  private renderBuildLink(invId: string) {
    const match = invId.match(/^build-(?<id>\d+)/);
    if (!match) {
      return '';
    }
    const buildPageUrl = router.urlForName('build-short-link', { build_id: match.groups!['id'] });
    return html`(<a href=${buildPageUrl} target="_blank">build page</a>)`;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: block;
        padding: 10px 20px;
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
    `,
  ];
}
