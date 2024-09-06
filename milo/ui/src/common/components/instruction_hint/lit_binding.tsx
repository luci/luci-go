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

import createCache, { EmotionCache } from '@emotion/cache';
import { CacheProvider } from '@emotion/react';
import { customElement } from 'lit/decorators.js';

import { ReactLitElement } from '@/generic_libs/components/react_lit_element';

import { InstructionHint } from './instruction_hint';

@customElement('milo-instruction-hint')
export class InstructionHintElement extends ReactLitElement {
  static get properties() {
    return {
      instructionName: {
        attribute: 'instruction-name',
        type: String,
      },
      title: {
        attribute: 'title',
        type: String,
      },
      placeholderData: {
        attribute: 'placeholder-data',
        type: Object,
      },
      litLink: {
        attribute: 'lit-link',
        type: Boolean,
      },
    };
  }

  private _instructionName = '';
  get instructionName() {
    return this._instructionName;
  }
  set instructionName(newVal: string) {
    if (newVal === this._instructionName) {
      return;
    }
    const oldVal = this._instructionName;
    this._instructionName = newVal;
    this.requestUpdate('instructionName', oldVal);
  }

  private _title = '';
  get title() {
    return this._title;
  }
  set title(newVal: string) {
    if (newVal === this._title) {
      return;
    }
    const oldVal = this._title;
    this._title = newVal;
    this.requestUpdate('title', oldVal);
  }

  private _placeholderData = {};
  get placeholderData() {
    return this._placeholderData;
  }
  set placeholderData(newVal: object) {
    if (newVal === this._placeholderData) {
      return;
    }
    const oldVal = this._placeholderData;
    this._placeholderData = newVal;
    this.requestUpdate('placeholderData', oldVal);
  }

  private _litLink = false;
  get litLink() {
    return this._litLink;
  }
  set litLink(newVal: boolean) {
    if (newVal === this._litLink) {
      return;
    }
    const oldVal = this._litLink;
    this._litLink = newVal;
    this.requestUpdate('litLink', oldVal);
  }
  private cache: EmotionCache | undefined = undefined;

  renderReact() {
    // When the the lit binding is used, it is very likely that
    // this element is in a shadow DOM. Provide emotion cache at
    // this level so the CSS styles are carried over.
    if (!this.cache) {
      this.cache = createCache({
        key: 'milo-instruction-hint',
        container: this,
      });
    }
    return (
      <CacheProvider value={this.cache}>
        <InstructionHint
          instructionName={this.instructionName}
          title={this.title}
          placeholderData={this.placeholderData}
          litLink={this.litLink}
        />
      </CacheProvider>
    );
  }
}
