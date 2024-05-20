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

import { html, LitElement, PropertyValueMap } from 'lit';
import { ReactNode } from 'react';

export const OPEN_PORTAL_EVENT = 'open-portal';
export const CLOSE_PORTAL_EVENT = 'close-portal';
export const ELEMENT_UPDATED_EVENT = 'element-updated';

/**
 * A class that helps building custom web component with React.
 *
 * Extends this class like `LitElement` except that
 * 1. use `.renderReact()` instead of `.render()` to render a component,
 * 2. the created custom web component can only be used under a
 *    `<PortalScope />`.
 */
export abstract class LitReactPortalElement extends LitElement {
  connectedCallback(): void {
    super.connectedCallback();
    this.dispatchEvent(
      new CustomEvent<LitReactPortalElement>(OPEN_PORTAL_EVENT, {
        detail: this,
        bubbles: true,
        composed: true,
      }),
    );
  }

  disconnectedCallback(): void {
    this.dispatchEvent(
      new CustomEvent<LitReactPortalElement>(CLOSE_PORTAL_EVENT, {
        detail: this,
        bubbles: true,
        composed: true,
      }),
    );
    super.disconnectedCallback();
  }

  protected render() {
    return html`<slot></slot>`;
  }

  protected updated(
    // Use the same type definition as the super class.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    changedProperties: PropertyValueMap<any> | Map<PropertyKey, unknown>,
  ): void {
    super.update(changedProperties);
    this.dispatchEvent(new CustomEvent(ELEMENT_UPDATED_EVENT));
  }

  /**
   * Similar to `.render()` but uses React node instead of lit-html template
   * to define the output content.
   *
   * In the real DOM tree, the content is rendered as children of this node in
   * the light DOM.
   * In the React virtual DOM tree, the content is rendered as children of the
   * nearest ancestor `<PortalScope />`.
   */
  abstract renderReact(): ReactNode;
}
