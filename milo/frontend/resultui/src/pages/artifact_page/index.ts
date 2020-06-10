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
import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { css, customElement, html, property } from 'lit-element';
import { autorun, computed, observable } from 'mobx';

import '../../components/status_bar';
import '../../components/tab_bar';
import { TabDef } from '../../components/tab_bar';
import { AppState, consumeAppState } from '../../context/app_state_provider';
import { router } from '../../routes';
import { ArtifactPageState, providePageState } from './context';


/**
 * Renders an Artifact.
 */
// TODO(weiweilin): improve error handling.
export class ArtifactPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  @property() pageState: ArtifactPageState = new ArtifactPageState();

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const artifactName = location.params['artifact_name'];
    if (typeof artifactName !== 'string') {
      return cmd.redirect('/not-found');
    }
    this.pageState.artifactName = artifactName;
    return;
  }

  private disposers: Array<() => void> = [];
  connectedCallback() {
    super.connectedCallback();
    this.disposers.push(autorun(
      () => this.pageState.appState = this.appState,
    ));
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
  }

  @computed get tabDefs(): TabDef[] {
    // TODO(weiweilin): add a details tab.
    return [
      {
        id: 'content',
        label: 'Content',
        href: router.urlForName(
          'artifact-content',
          {'artifact_name': this.pageState.artifactName},
        ),
      },
    ];
  }

  protected render() {
    return html`
      <div id="artifact-header">
        <div id="artifact-id">
          <span id="artifact-id-label">Artifact ID</span>
          <span>${this.pageState.artifact?.artifactId ?? 'loading'}</span>
        </div>
      </div>
      <tr-status-bar
        .components=${[{color: '#007bff', weight: 1}]}
        .loading=${this.pageState.artifactRes.state === 'pending'}
      ></tr-status-bar>
      <tr-tab-bar
        .tabs=${this.tabDefs}
        .selectedTabId=${this.pageState.selectedTabId}
      ></tr-tab-bar>
      <slot></slot>
    `;
  }

  static styles = css`
    #artifact-header {
      background-color: rgb(248, 249, 250);
      padding: 6px 16px;
      font-family: "Google Sans", "Helvetica Neue", sans-serif;
      font-size: 14px;
      display: flex;
    }
    #artifact-id {
      flex: 0 auto;
    }
    #artifact-id-label {
      color: rgb(95, 99, 104);
    }
  `;
}

customElement('tr-artifact-page')(
  providePageState(
    consumeAppState(
      ArtifactPageElement,
    ),
  ),
);
