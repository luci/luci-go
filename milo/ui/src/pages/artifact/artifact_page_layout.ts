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

import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable, reaction } from 'mobx';

import '../../components/image_diff_viewer';
import '../../components/status_bar';
import { MiloBaseElement } from '../../components/milo_base';
import { createContextLink, provider } from '../../libs/context';
import { getInvURLPath, NOT_FOUND_URL } from '../../libs/url_utils';
import { ArtifactIdentifier } from '../../services/resultdb';
import commonStyle from '../../styles/common_style.css';

export const [provideArtifactIdent, consumeArtifactIdent] = createContextLink<ArtifactIdentifier>();

/**
 * Renders the header of an artifact page.
 */
@customElement('milo-artifact-page-layout')
@provider
export class ArtifactPageLayoutElement extends MiloBaseElement implements BeforeEnterObserver {
  @observable.ref private invocationId!: string;
  @observable.ref private testId?: string;
  @observable.ref private resultId?: string;
  @observable.ref private artifactId!: string;

  @computed
  @provideArtifactIdent()
  get artifactIdent() {
    return {
      invocationId: this.invocationId,
      testId: this.testId,
      resultId: this.resultId,
      artifactId: this.artifactId,
    };
  }

  constructor() {
    super();
    makeObservable(this);
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const invocationId = location.params['inv_id'];
    const testId = location.params['test_id'];
    const resultId = location.params['result_id'];
    const artifactId = location.params['artifact_id'];

    if ([invocationId, testId || '', resultId || '', artifactId].some((param) => typeof param !== 'string')) {
      return cmd.redirect(NOT_FOUND_URL);
    }

    this.invocationId = invocationId as string;
    this.testId = testId as string | undefined;
    this.resultId = resultId as string | undefined;
    this.artifactId = artifactId as string;
    return;
  }

  connectedCallback() {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.artifactIdent,
        (artifactIdent) => {
          // Emulate @property() update.
          this.updated(new Map([['artifactIdent', artifactIdent]]));
        },
        { fireImmediately: true }
      )
    );
  }

  protected render() {
    return html`
      <div id="artifact-header">
        <table>
          <tr>
            <td class="id-component-label">Invocation</td>
            <td>
              <a href=${getInvURLPath(this.invocationId)}>${this.invocationId}</a>
            </td>
          </tr>
          ${this.testId &&
          html`
            <!-- TODO(weiweilin): add view test link -->
            <tr>
              <td class="id-component-label">Test</td>
              <td>${this.testId}</td>
            </tr>
          `}
          ${this.resultId &&
          html`
            <!-- TODO(weiweilin): add view result link -->
            <tr>
              <td class="id-component-label">Result</td>
              <td>${this.resultId}</td>
            </tr>
          `}
          <tr>
            <td class="id-component-label">Artifact</td>
            <td>${this.artifactId}</td>
          </tr>
        </table>
      </div>
      <milo-status-bar .components=${[{ color: 'var(--active-color)', weight: 1 }]}></milo-status-bar>
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
