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
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import '@/generic_libs/components/dot_spinner';
import { ARTIFACT_LENGTH_LIMIT } from '@/common/constants/test';
import { Artifact } from '@/common/services/resultdb';
import { commonStyles } from '@/common/styles/stylesheets';
import { reportRenderError } from '@/generic_libs/tools/error_handler';
import { consumer } from '@/generic_libs/tools/lit_context';
import { unwrapObservable } from '@/generic_libs/tools/mobx_utils';
import { urlSetSearchQueryParam } from '@/generic_libs/tools/utils';

import {
  consumeArtifacts,
  consumeArtifactsFinalized,
} from './artifact_provider';

/**
 * Renders a text artifact.
 */
@customElement('text-artifact')
@consumer
export class TextArtifactElement extends MobxLitElement {
  static get properties() {
    return {
      artifactId: {
        attribute: 'artifact-id',
        type: String,
      },
      invLevel: {
        attribute: 'inv-level',
        type: Boolean,
      },
    };
  }

  @observable.ref
  @consumeArtifacts()
  artifacts!: Map<string, Artifact>;

  @observable.ref
  @consumeArtifactsFinalized()
  finalized = false;

  @observable.ref _artifactId!: string;
  @computed get artifactId() {
    return this._artifactId;
  }
  set artifactId(newVal: string) {
    this._artifactId = newVal;
  }

  @observable.ref _invLevel = false;
  @computed get invLevel() {
    return this._invLevel;
  }
  set invLevel(newVal: boolean) {
    this._invLevel = newVal;
  }

  @computed
  private get fetchUrl(): string {
    const artifact = this.artifacts.get(
      (this.invLevel ? 'inv-level/' : '') + this.artifactId,
    );
    // TODO(crbug/1206109): use permanent raw artifact URL.
    return artifact ? artifact.fetchUrl : '';
  }

  @computed
  private get content$(): IPromiseBasedObservable<string> {
    if (!this.fetchUrl) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(
      fetch(
        urlSetSearchQueryParam(this.fetchUrl, 'n', ARTIFACT_LENGTH_LIMIT),
      ).then((res) => res.text()),
    );
  }

  @computed
  private get content() {
    return unwrapObservable(this.content$, null);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render = reportRenderError(this, () => {
    const label = this._invLevel ? 'Inv-level artifact' : 'Artifact';

    if (this.finalized && this.fetchUrl === '') {
      return html`<div>${label}: <i>${this.artifactId}</i> not found.</div>`;
    }

    if (this.content === null) {
      return html`<div id="load">
        Loading <milo-dot-spinner></milo-dot-spinner>
      </div>`;
    }

    if (this.content === '') {
      return html`<div>${label}: <i>${this.artifactId}</i> is empty.</div>`;
    }

    return html`<pre>${this.content}</pre>`;
  });

  static styles = [
    commonStyles,
    css`
      #load {
        color: var(--active-text-color);
      }
      pre {
        margin: 0;
        font-size: 12px;
        white-space: pre-wrap;
        overflow-wrap: anywhere;
      }
    `,
  ];
}
