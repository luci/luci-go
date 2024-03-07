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
import { commonStyles } from '@/common/styles/stylesheets';
import { unwrapObservable } from '@/generic_libs/tools/mobx_utils';
import { toError, urlSetSearchQueryParam } from '@/generic_libs/tools/utils';

import { ON_TEST_RESULT_DATA_READY } from '../constants/event';
import { getRawArtifactURLPath } from '../tools/url_utils';

export interface TextArtifactEvent {
  setData: (invName: string, resultName: string) => void;
}

/**
 * Renders a text artifact.
 */
@customElement('text-artifact')
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
  resultName: string | undefined;

  @observable.ref
  invName: string | undefined;

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
    if (!this.resultName || !this.invName) {
      return '';
    }

    if (this.invLevel) {
      return getRawArtifactURLPath(
        `${this.invName}/artifacts/${this.artifactId}`,
      );
    }

    return getRawArtifactURLPath(
      `${this.resultName}/artifacts/${this.artifactId}`,
    );
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

  eventTimeout: number | undefined;

  constructor() {
    super();
    makeObservable(this);
  }

  protected createRenderRoot() {
    return this;
  }

  connectedCallback(): void {
    super.connectedCallback();
    // This must be done in the connected callback as it needs
    // to occur when the element is in the dom in order for the
    // event to bubble up to the parent.
    // The timeout is needed to ensure that the event is fired after
    // parent React components are mounted and were able to
    // register the event handlers
    this.eventTimeout = window.setTimeout(
      () =>
        this.dispatchEvent(
          new CustomEvent<TextArtifactEvent>(ON_TEST_RESULT_DATA_READY, {
            detail: {
              setData: (invName, resultName) => {
                this.resultName = resultName;
                this.invName = invName;
                this.requestUpdate();
              },
            },
            // Allows events to bubble up the tree,
            // which can then be captured by React components.
            bubbles: true,
            // Allows the event to traverse shadowroot boundries.
            composed: true,
          }),
        ),
      0,
    );
  }

  disconnectedCallback(): void {
    super.disconnectedCallback();

    clearTimeout(this.eventTimeout);
  }

  protected render() {
    try {
      const label = this._invLevel ? 'Inv-level artifact' : 'Artifact';

      if (this.content === null) {
        return html`<div id="load">
          Loading <milo-dot-spinner></milo-dot-spinner>
        </div>`;
      }

      if (this.content === '') {
        return html`<div>${label}: <i>${this.artifactId}</i> is empty.</div>`;
      }

      return html`<pre>${this.content}</pre>`;
    } catch (e: unknown) {
      const error = toError(e);
      return html`<span>An error occured: ${error.message}</span>`;
    }
  }

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
