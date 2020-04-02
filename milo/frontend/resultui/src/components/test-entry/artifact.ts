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
import { observable } from 'mobx';
import { Artifact } from '../../services/resultdb';

/**
 * Renders the given artifact. Includes:
 *  - a display link.
 *  - inline rendered content for plain/text or image/*.
 *  - a download link if there's one.
 */
@customElement('tr-artifact')
export class ArtifactElement extends MobxLitElement {
  @observable.ref
  artifact?: Artifact;

  /**
   * Renders the content of the artifact.
   */
  private renderContent() {
    if (this.artifact!.contentType === 'plain/text') {
      return this.artifact!.contents;
    } else if (this.artifact!.contentType?.startsWith('image/')) {
      return this.artifact!.contents;
    } else {
      return;
    }
  }

  // TODO(weiweilin): implement image/text diff view.
  protected render() {
    return html`
      <a href=${this.artifact!.viewUrl}>${this.artifact!.name}</a>
      ${this.artifact!.fetchUrl && html`<a href=${this.artifact!.fetchUrl}><mwc-icon class="inline-icon">save_alt</mwc-icon></a>`}
      ${this.renderContent()}
    `;
  }

  static styles = css`
    .inline-icon {
      --mdc-icon-size: 1em;
      top: .125em;
      position: relative;
      color: black;
    }
  `;
}
