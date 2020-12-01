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
import '@material/mwc-icon';
import { css, customElement, html, property } from 'lit-element';
import { computed, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';
import { consumeContext } from '../../libs/context';
import { Artifact } from '../../services/resultdb';

/**
 * Renders a text artifact.
 */
export class TextArtifactElement extends MobxLitElement {
  @property({attribute: 'artifact-id'}) artifactID!: string;
  @observable.ref artifacts!: Map<string, Artifact>;

  @computed
  private get fetchUrl(): string|undefined {
    const artifact = this.artifacts.get(this.artifactID);
    return artifact ? artifact.fetchUrl : '';
  }

  @computed
  private get contentRes(): IPromiseBasedObservable<string> {
    if (!this.fetchUrl) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(fetch(this.fetchUrl).then((res) => res.text()));
  }

  @computed
  private get content() {
    return this.contentRes.state === 'fulfilled' ? this.contentRes.value : '';
  }

  protected render() {
    if (!this.content) {
      return html`
      <div id="load">
        Loading <milo-dot-spinner></milo-dot-spinner>
      </div>
      `;
    }
    const c = `/b/s/w/ir/out/Release/content_shell --disable-site-isolation-trials --run-web-tests --ignore-certificate-errors-spki-list=Nxvaj3+bY3oVrTc+Jp7m3E3sB1n3lXtnMDCyBsqEXiY=,55qC1nKu2A88ESbFmk5sTPQS/ScG+8DD7P+2bgFA9iM=,0Rt4mT6SJXojEMHTnKnlJ/hBKMBcI4kteBlhR1eTTdk= --user-data-dir --enable-crash-reporter --crash-dumps-dir=/b/s/w/ir/out/Release/crash-dumps - /b/s/w/ir/third_party/blink/web_tests/bluetooth/descriptor/writeValue/gen-io-op-garbage-collection-ran-during-error.html`;
    // return html`
    //     <pre>${this.content}</pre>
    // `;
    return html`
        <pre>${c}</pre>
    `;
  }

  static styles = css`
    #load {
      color: var(--active-text-color);
    }
    pre {
      white-space: pre-wrap;
    }
  `;
}

customElement('text-artifact') (
  consumeContext<'artifacts', Map<string, Artifact>>('artifacts')(TextArtifactElement),
);
