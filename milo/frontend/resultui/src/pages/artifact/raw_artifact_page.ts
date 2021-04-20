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

import { css, customElement, html } from 'lit-element';
import { autorun, computed, observable } from 'mobx';
import { fromPromise } from 'mobx-utils';

import '../../components/dot_spinner';
import '../../components/status_bar';
import { reportError } from '../../components/error_handler';
import { MiloBaseElement } from '../../components/milo_base';
import { AppState, consumeAppState } from '../../context/app_state';
import { consumeContext } from '../../libs/context';
import { unwrapObservable } from '../../libs/utils';
import { ArtifactIdentifier, constructArtifactName } from '../../services/resultdb';
import commonStyle from '../../styles/common_style.css';

/**
 * Renders a raw artifact.
 */
@customElement('milo-raw-artifact-page')
@consumeAppState
@consumeContext('artifactIdent')
export class RawArtifactPageElement extends MiloBaseElement {
  @observable.ref appState!: AppState;
  @observable.ref artifactIdent!: ArtifactIdentifier;

  @computed
  private get artifact$() {
    if (!this.appState.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getArtifact({ name: constructArtifactName(this.artifactIdent) }));
  }

  @computed
  private get artifact() {
    return unwrapObservable(this.artifact$, null);
  }

  connectedCallback() {
    super.connectedCallback();

    this.addDisposer(
      autorun(
        reportError.bind(this)(() => {
          if (this.artifact) {
            window.open(this.artifact.fetchUrl, '_self');
          }
        })
      )
    );
  }

  protected render() {
    return html`<div id="content">Loading artifact <milo-dot-spinner></milo-dot-spinner></div>`;
  }

  static styles = [
    commonStyle,
    css`
      #content {
        margin: 20px;
        color: var(--active-color);
      }
    `,
  ];
}
