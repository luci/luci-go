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
import { customElement, html } from 'lit-element';
import { computed, makeObservable, observable } from 'mobx';

import '../expandable_entry';
import '../image_diff_viewer';
import { router } from '../../routes';
import { Artifact } from '../../services/resultdb';

/**
 * Renders an image diff artifact entry.
 */
@customElement('milo-image-diff-artifact')
export class TextDiffArtifactElement extends MobxLitElement {
  @observable.ref expected!: Artifact;
  @observable.ref actual!: Artifact;
  @observable.ref diff!: Artifact;

  @computed private get artifactPageUrl() {
    const search = new URLSearchParams();
    search.set('actual_artifact_id', this.actual.artifactId);
    search.set('expected_artifact_id', this.expected.artifactId);
    return `${router.urlForName('artifact')}/image-diff/${this.diff.name}?${search}`;
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    return html`
      <milo-expandable-entry .expanded=${true} .contentRuler="invisible">
        <span id="header" slot="header">
          Unexpected image output from
          <a href=${this.artifactPageUrl} target="_blank">${this.diff.artifactId}</a>
        </span>
        <milo-image-diff-viewer slot="content" .expected=${this.expected} .actual=${this.actual} .diff=${this.diff}>
        </milo-image-diff-viewer>
      </milo-expandable-entry>
    `;
  }
}
