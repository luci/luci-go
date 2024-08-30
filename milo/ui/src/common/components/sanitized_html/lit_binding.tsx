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

import { SanitizedHtml } from './sanitized_html';

/**
 * Sanitize & render the HTML into a <Box />. It's a lit binding for the React
 * <SanitizedHtml /> component.
 *
 * Note that this should be used even when a trusted type policy is used. This
 * adds another layer of defense in case the trusted type policy is not active.
 * This may happen when
 * 1. The trusted type policy is misconfigured.
 * 2. A proper CSP context is not set via the HTTP header or HTML meta tag.
 *
 * It also makes components under unit test behave closer production. As trusted
 * type policy and CSP are not typically used in unit tests.
 */
@customElement('milo-sanitized-html')
export class SanitizedHtmlElement extends ReactLitElement {
  static get properties() {
    return {
      html: {
        attribute: 'html',
        type: String,
      },
    };
  }

  private _html = '';
  get html() {
    return this._html;
  }
  set html(newVal: string) {
    if (newVal === this._html) {
      return;
    }
    const oldVal = this._html;
    this._html = newVal;
    this.requestUpdate('html', oldVal);
  }

  private cache: EmotionCache | undefined = undefined;

  renderReact() {
    // When the the lit binding is used, it is very likely that
    // this element is in a shadow DOM. Provide emotion cache at
    // this level so the CSS styles are carried over.
    if (!this.cache) {
      this.cache = createCache({
        key: 'milo-sanitized-html',
        container: this,
      });
    }

    return (
      <CacheProvider value={this.cache}>
        <SanitizedHtml html={this.html} />
      </CacheProvider>
    );
  }
}
