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

import * as ace from 'ace-builds';
import 'ace-builds/src-noconflict/mode-json';
import { css, customElement, LitElement, property } from 'lit-element';
import { html } from 'lit-html';

ace.config.setModuleUrl('ace/mode/json_worker', require('file-loader!ace-builds/src-noconflict/worker-json').default);

/**
 * A lit-element wrapper of ace-editor.
 */
@customElement('milo-ace-editor')
export class AceEditorEntryElement extends LitElement {
  @property() options?: Partial<ace.Ace.EditorOptions>;

  private editor!: ace.Ace.Editor;

  protected render() {
    return html`<div id="editor"></div>`;
  }

  protected firstUpdated() {
    this.editor = ace.edit(this.shadowRoot!.getElementById('editor')!);
    this.editor.renderer.attachToShadowRoot();
  }

  protected updated() {
    this.editor.setOptions(this.options || {});
  }

  static styles = css`
    :host {
      display: block;
      border-radius: 4px;
      border: 1px solid var(--divider-color);
    }
    #editor {
      position: relative;
    }
  `;
}
