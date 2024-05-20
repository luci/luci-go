// Copyright 2024 The LUCI Authors.
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

import { html, LitElement } from 'lit';
import { customElement } from 'lit/decorators.js';

import { createContextLink, provider } from '@/generic_libs/tools/lit_context';

const [provideResultName, _consumeResultName] = createContextLink<string>();
export const consumeResultName = _consumeResultName;

/**
 * Provides `resultName` context for rendering artifact tag in a Lit element.
 *
 * Other React contexts used by the artifact tags must be available to the
 * nearest ancestor `<PortalScope />`.
 */
@customElement('milo-artifact-tag-context-provider')
@provider
export class ArtifactTagContextProviderElement extends LitElement {
  static get properties() {
    return {
      resultName: {
        attribute: 'result-name',
        type: String,
      },
    };
  }

  private _resultName = '';
  @provideResultName()
  get resultName() {
    return this._resultName;
  }
  set resultName(newVal: string) {
    if (newVal === this._resultName) {
      return;
    }
    const oldVal = this._resultName;
    this._resultName = newVal;
    this.requestUpdate('resultName', oldVal);
  }

  protected render() {
    return html`<slot></slot>`;
  }
}
