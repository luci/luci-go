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
import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { BeforeEnterObserver, PreventAndRedirectCommands, Router, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { autorun, computed, observable } from 'mobx';
import { fromPromise, PENDING, REJECTED } from 'mobx-utils';

import '../../components/dot_spinner';
import '../../components/status_bar';
import { AppState, consumeAppState } from '../../context/app_state';
import { NOT_FOUND_URL, router } from '../../routes';
import { parseArtifactName } from '../../services/resultdb';

/**
 * Renders a raw artifact.
 */
// TODO(weiweilin): improve error handling.
@customElement('milo-raw-artifact-page')
@consumeAppState
export class RawArtifactPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  @observable.ref private artifactName!: string;

  @computed private get artifactIdent() {
    return parseArtifactName(this.artifactName);
  }

  @computed
  private get artifact$() {
    if (!this.appState.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getArtifact({ name: this.artifactName }));
  }

  private disposers: Array<() => void> = [];

  connectedCallback() {
    super.connectedCallback();

    // TODO(weiweilin): add integration tests to ensure redirection works properly.
    this.disposers.push(
      autorun(() => {
        if (this.artifact$.state === PENDING) {
          return;
        }

        if (this.artifact$.state === REJECTED) {
          const err = this.artifact$.value as GrpcError;
          const mayRequireSignin = [RpcCode.NOT_FOUND, RpcCode.PERMISSION_DENIED, RpcCode.UNAUTHENTICATED].includes(
            err.code
          );
          if (mayRequireSignin && this.appState.userId === '') {
            Router.go(`${router.urlForName('login')}?${new URLSearchParams([['redirect', window.location.href]])}`);
            return;
          }
          this.dispatchEvent(
            new ErrorEvent('error', {
              message: err.message,
              composed: true,
              bubbles: true,
            })
          );
          return;
        }

        window.open(this.artifact$.value.fetchUrl, '_self');
      })
    );
  }

  disconnectedCallback() {
    for (const disposer of this.disposers) {
      disposer();
    }
    super.disconnectedCallback();
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const artifactName = location.params['artifact_name'];
    if (typeof artifactName !== 'string') {
      return cmd.redirect(NOT_FOUND_URL);
    }
    this.artifactName = artifactName;
    return;
  }

  protected render() {
    return html`
      <div id="artifact-header">
        <table>
          <tr>
            <td class="id-component-label">Invocation</td>
            <td>
              <a href=${router.urlForName('invocation', { invocation_id: this.artifactIdent.invocationId })}
                >${this.artifactIdent.invocationId}</a
              >
            </td>
          </tr>
          ${this.artifactIdent.testId &&
          html`
            <!-- TODO(weiweilin): add view test link -->
            <tr>
              <td class="id-component-label">Test</td>
              <td>${this.artifactIdent.testId}</td>
            </tr>
          `}
          ${this.artifactIdent.resultId &&
          html`
            <!-- TODO(weiweilin): add view result link -->
            <tr>
              <td class="id-component-label">Result</td>
              <td>${this.artifactIdent.resultId}</td>
            </tr>
          `}
          <tr>
            <td class="id-component-label">Artifact</td>
            <td>${this.artifactIdent.artifactId}</td>
          </tr>
        </table>
      </div>
      <milo-status-bar
        .components=${[{ color: 'var(--active-color)', weight: 1 }]}
        .loading=${this.artifact$.state === 'pending'}
      ></milo-status-bar>
      <div id="content">Loading artifact <milo-dot-spinner></milo-dot-spinner></div>
    `;
  }

  static styles = css`
    #artifact-header {
      background-color: var(--block-background-color);
      padding: 6px 16px;
      font-family: 'Google Sans', 'Helvetica Neue', sans-serif;
      font-size: 14px;
    }

    #content {
      margin: 20px;
      color: var(--active-color);
    }
  `;
}
