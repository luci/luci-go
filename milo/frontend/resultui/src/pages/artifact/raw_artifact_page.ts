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
import { autorun, computed, makeObservable, observable } from 'mobx';
import { fromPromise } from 'mobx-utils';

import '../../components/dot_spinner';
import '../../components/status_bar';
import { MiloBaseElement } from '../../components/milo_base';
import { consumer } from '../../libs/context';
import { reportError, reportRenderError } from '../../libs/error_handler';
import { unwrapObservable } from '../../libs/unwrap_observable';
import { ArtifactIdentifier, constructArtifactName } from '../../services/resultdb';
import { consumeStore, StoreInstance } from '../../store';
import commonStyle from '../../styles/common_style.css';
import { consumeArtifactIdent } from './artifact_page_layout';

/**
 * Renders a raw artifact.
 */
@customElement('milo-raw-artifact-page')
@consumer
export class RawArtifactPageElement extends MiloBaseElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref
  @consumeArtifactIdent()
  artifactIdent!: ArtifactIdentifier;

  @computed
  private get artifact$() {
    if (!this.store.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.store.resultDb.getArtifact({ name: constructArtifactName(this.artifactIdent) }));
  }

  @computed
  private get artifact() {
    return unwrapObservable(this.artifact$, null);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();

    /**
     * TODO(crbug/1206109): artifact.fetchUrl has an expire time. Ideally, we
     * want to handle the request on the server side, so we can avoid
     * redirecting to a temporary URL (users may save the URL). However, due to
     * a bug in golang http/net, we can't handle this on server side before
     * migrating to GAE v2.
     */
    this.addDisposer(
      autorun(
        reportError(this, () => {
          if (this.artifact) {
            window.open(this.artifact.fetchUrl, '_self');
          }
        })
      )
    );
  }

  protected render = reportRenderError(this, () => {
    if (!this.artifact) {
      return html`<div id="content" class="active-text">Loading artifact <milo-dot-spinner></milo-dot-spinner></div>`;
    }
    // The page should've navigated to the artifact fetch URL, but the browser
    // may decide to download it instead of rendering it as a page.
    // Render a download link in that case.
    return html`
      <div id="content">
        The artifact content can not be directly rendered as a page.<br />
        The browser should start downloading the artifact momentarily.<br />
        If not, you can use the following link to download the artifact.<br />
        <a href=${this.artifact.fetchUrl}>${this.artifact.fetchUrl}</a>
      </div>
    `;
  });

  static styles = [
    commonStyle,
    css`
      #content {
        margin: 20px;
      }
    `,
  ];
}
