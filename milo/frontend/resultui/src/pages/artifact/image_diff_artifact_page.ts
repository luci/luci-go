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
import { css, customElement, html } from 'lit-element';
import { computed, makeObservable, observable } from 'mobx';
import { fromPromise } from 'mobx-utils';

import '../../components/image_diff_viewer';
import '../../components/status_bar';
import '../../components/dot_spinner';
import { AppState, consumeAppState } from '../../context/app_state';
import { consumer } from '../../libs/context';
import { reportRenderError } from '../../libs/error_handler';
import { unwrapObservable } from '../../libs/unwrap_observable';
import { NOT_FOUND_URL } from '../../routes';
import { ArtifactIdentifier, constructArtifactName } from '../../services/resultdb';
import commonStyle from '../../styles/common_style.css';
import { consumeArtifactIdent } from './artifact_page_layout';

/**
 * Renders an image diff artifact set, including expected image, actual image
 * and image diff.
 */
// TODO(weiweilin): improve error handling.
@customElement('milo-image-diff-artifact-page')
@consumer
export class ImageDiffArtifactPage extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref
  @consumeAppState()
  appState!: AppState;

  @observable.ref
  @consumeArtifactIdent()
  artifactIdent!: ArtifactIdentifier;

  @computed private get diffArtifactName() {
    return constructArtifactName({ ...this.artifactIdent });
  }
  @computed private get expectedArtifactName() {
    return constructArtifactName({ ...this.artifactIdent, artifactId: this.expectedArtifactId });
  }
  @computed private get actualArtifactName() {
    return constructArtifactName({ ...this.artifactIdent, artifactId: this.actualArtifactId });
  }

  @observable.ref private expectedArtifactId!: string;
  @observable.ref private actualArtifactId!: string;

  @computed
  private get diffArtifact$() {
    if (!this.appState.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getArtifact({ name: this.diffArtifactName }));
  }
  @computed private get diffArtifact() {
    return unwrapObservable(this.diffArtifact$, null);
  }

  @computed
  private get expectedArtifact$() {
    if (!this.appState.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getArtifact({ name: this.expectedArtifactName }));
  }
  @computed private get expectedArtifact() {
    return unwrapObservable(this.expectedArtifact$, null);
  }

  @computed
  private get actualArtifact$() {
    if (!this.appState.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getArtifact({ name: this.actualArtifactName }));
  }
  @computed private get actualArtifact() {
    return unwrapObservable(this.actualArtifact$, null);
  }

  @computed get isLoading() {
    return !this.expectedArtifact || !this.actualArtifact || !this.diffArtifact;
  }

  constructor() {
    super();
    makeObservable(this);
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const search = new URLSearchParams(location.search);
    const expectedArtifactId = search.get('expected_artifact_id');
    const actualArtifactId = search.get('actual_artifact_id');

    if (!expectedArtifactId || !actualArtifactId) {
      return cmd.redirect(NOT_FOUND_URL);
    }

    this.expectedArtifactId = expectedArtifactId;
    this.actualArtifactId = actualArtifactId;
    return;
  }

  protected render = reportRenderError(this, () => {
    if (this.isLoading) {
      return html`<div id="loading-spinner" class="active-text">Loading <milo-dot-spinner></milo-dot-spinner></div>`;
    }

    return html`
      <milo-image-diff-viewer
        .expected=${this.expectedArtifact}
        .actual=${this.actualArtifact}
        .diff=${this.diffArtifact}
      >
      </milo-image-diff-viewer>
    `;
  });

  static styles = [
    commonStyle,
    css`
      :host {
        display: block;
      }

      #loading-spinner {
        margin: 20px;
      }
    `,
  ];
}
