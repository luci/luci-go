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

import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement, html } from 'lit-element';
import { observable } from 'mobx';

import '../../components/image_diff_viewer';
import '../../components/status_bar';
import { router } from '../../routes';
import { ArtifactIdentifier } from '../../services/resultdb';
import commonStyle from '../../styles/common_style.css';

/**
 * Renders the header of an artifact page.
 */
@customElement('milo-artifact-page-layout')
export class ArtifactPageLayoutElement extends MobxLitElement {
  @observable.ref ident!: ArtifactIdentifier;
  // This is only needed when there are multiple artifact IDs.
  @observable.ref artifactIds?: string[];
  @observable.ref isLoading = false;

  protected render() {
    return html`
      <div id="artifact-header">
        <table>
          <tr>
            <td class="id-component-label">Invocation</td>
            <td>
              <a
                href=${router.urlForName('invocation', {
                  invocation_id: this.ident.invocationId,
                })}
              >
                ${this.ident.invocationId}
              </a>
            </td>
          </tr>
          ${this.ident.testId &&
          html`
            <!-- TODO(weiweilin): add view test link -->
            <tr>
              <td class="id-component-label">Test</td>
              <td>${this.ident.testId}</td>
            </tr>
          `}
          ${this.ident.resultId &&
          html`
            <!-- TODO(weiweilin): add view result link -->
            <tr>
              <td class="id-component-label">Result</td>
              <td>${this.ident.resultId}</td>
            </tr>
          `}
          <tr>
            <td class="id-component-label">Artifacts</td>
            <td>${this.artifactIds?.join(', ') || this.ident.artifactId}</td>
          </tr>
        </table>
      </div>
      <milo-status-bar
        .components=${[{ color: 'var(--active-color)', weight: 1 }]}
        .loading=${this.isLoading}
      ></milo-status-bar>
      <slot></slot>
    `;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: block;
      }

      #artifact-header {
        background-color: var(--block-background-color);
        padding: 6px 16px;
        font-family: 'Google Sans', 'Helvetica Neue', sans-serif;
        font-size: 14px;
      }
      .id-component-label {
        color: var(--light-text-color);
      }
    `,
  ];
}
