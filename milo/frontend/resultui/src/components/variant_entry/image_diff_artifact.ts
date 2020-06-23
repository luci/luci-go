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
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable } from 'mobx';

import { router } from '../../routes';
import { Artifact } from '../../services/resultdb';
import '../image_diff_viewer';

/**
 * Renders an image diff artifact entry.
 */
@customElement('tr-image-diff-artifact')
export class TextDiffArtifactElement extends MobxLitElement {
  @observable.ref expected!: Artifact;
  @observable.ref actual!: Artifact;
  @observable.ref diff!: Artifact;

  @observable.ref private expanded = true;

  @computed private get artifactPageUrl() {
    const search = new URLSearchParams();
    search.set('actual_artifact_id', this.actual.artifactId);
    search.set('expected_artifact_id', this.expected.artifactId);
    return `${router.urlForName('image-diff-artifact', {'artifact_name': this.diff.name})}?${search}`;
  }

  protected render() {
    return html`
      <div
        class="expandable-header"
        @click=${() => this.expanded = !this.expanded}
      >
        <mwc-icon class="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <span class="one-line-content">
          Unexpected image output from
          <a href=${this.artifactPageUrl} target="_blank">${this.diff.artifactId}</a>
        </span>
      </div>
      <div id="container" style=${styleMap({display: this.expanded ? '' : 'none'})}>
        <tr-image-diff-viewer
          .expected=${this.expected}
          .actual=${this.actual}
          .diff=${this.diff}
        >
        </tr-image-diff-viewer>
      </div>
    `;
  }

  static styles = css`
    .expandable-header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: 24px;
      grid-gap: 5px;
      cursor: pointer;
    }
    .expandable-header .expand-toggle {
      grid-row: 1;
      grid-column: 1;
    }
    .expandable-header .one-line-content {
      grid-row: 1;
      grid-column: 2;
      line-height: 24px;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }
    #entry-header .one-line-content {
      font-size: 14px;
      letter-spacing: 0.1px;
      font-weight: 500;
    }

    #container {
      padding: 0 10px;
      margin: 5px;
    }
  `;
}
