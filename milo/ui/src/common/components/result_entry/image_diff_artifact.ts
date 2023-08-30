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
import { html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable } from 'mobx';

import '@/generic_libs/components/expandable_entry';
import '@/common/components/image_diff_viewer';
import { Artifact } from '@/common/services/resultdb';
import { getImageDiffArtifactURLPath } from '@/common/tools/url_utils';

/**
 * Renders an image diff artifact entry.
 */
@customElement('milo-image-diff-artifact')
export class TextDiffArtifactElement extends MobxLitElement {
  @observable.ref expected!: Artifact;
  @observable.ref actual!: Artifact;
  @observable.ref diff!: Artifact;

  @computed private get artifactPageUrl() {
    return getImageDiffArtifactURLPath(
      this.diff.name,
      this.actual.artifactId,
      this.expected.artifactId,
    );
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
          <a href=${this.artifactPageUrl} target="_blank"
            >${this.diff.artifactId}</a
          >
        </span>
        <milo-image-diff-viewer
          slot="content"
          .expected=${this.expected}
          .actual=${this.actual}
          .diff=${this.diff}
        >
        </milo-image-diff-viewer>
      </milo-expandable-entry>
    `;
  }
}
