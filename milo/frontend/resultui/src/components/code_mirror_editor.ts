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

import * as CodeMirror from 'codemirror';
import 'codemirror/addon/edit/matchbrackets';
import 'codemirror/addon/fold/brace-fold';
import 'codemirror/addon/fold/foldcode';
import 'codemirror/addon/fold/foldgutter';
import 'codemirror/mode/javascript/javascript';
import { css, customElement, LitElement } from 'lit-element';
import { html } from 'lit-html';
import { observable } from 'mobx';

const foldGutterStyle = require('codemirror/addon/fold/foldgutter.css').default;
const codemirrorStyle = require('codemirror/lib/codemirror.css').default;

/**
 * A lit-element wrapper of codemirror
 */
@customElement('milo-code-mirror-editor')
export class CodeMirrorEditorElement extends LitElement {
  @observable.ref value!: string;
  @observable.ref options: CodeMirror.EditorConfiguration | undefined;
  onInit = (_editor: CodeMirror.Editor) => {};

  protected firstUpdated() {
    const editor = CodeMirror.fromTextArea(
      this.shadowRoot!.getElementById('editor') as HTMLTextAreaElement,
      this.options
    );
    this.onInit(editor);
  }

  protected render() {
    return html` <textarea id="editor">${this.value}</textarea> `;
  }

  static styles = [
    codemirrorStyle,
    foldGutterStyle,
    css`
      :host {
        display: block;
        border-radius: 4px;
        border: 1px solid var(--divider-color);
      }
      .CodeMirror {
        height: auto;
        max-height: 1000px;
        font-size: 12px;
      }
      .CodeMirror-scroll {
        max-height: 1000px;
      }
      .cm-property.cm-string {
        color: #318495;
      }
      .cm-string:not(.cm-property) {
        color: #036a06;
      }

      .CodeMirror-vscrollbar::-webkit-scrollbar {
        opacity: 0;
        -webkit-appearance: none;
        width: 7px;
      }

      .CodeMirror-vscrollbar::-webkit-scrollbar-thumb {
        opacity: 0;
        border-radius: 4px;
        background-color: rgba(0, 0, 0, 0.5);
        box-shadow: 0 0 1px rgba(255, 255, 255, 0.5);
      }

      .CodeMirror .CodeMirror-vscrollbar {
        opacity: 0;
        transition-duration: 0.5s;
      }

      .CodeMirror:hover .CodeMirror-vscrollbar {
        opacity: 1;
      }
    `,
  ];
}
