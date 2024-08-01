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
import { Dispatch, ReactNode, SetStateAction, useState } from 'react';
import { createPortal } from 'react-dom';

export const ATTACHED_REACT_LIT_ELEMENT_EVENT = 'attached-react-lit-element';
export const DETACHED_REACT_LIT_ELEMENT_EVENT = 'deteached-react-lit-element';

const USE_REACT_NODE = Symbol('use-react-node');

/**
 * A class that helps building custom web component with React.
 *
 * Extends this class like `LitElement` except that
 * 1. use `.renderReact()` instead of `.render()` to render a component,
 * 2. the created custom web component can only be used in a
 *    `<ReactLitBridge />`.
 */
export abstract class ReactLitElement extends LitElement {
  private setState: Dispatch<SetStateAction<Record<string, never>>> | null =
    null;

  connectedCallback(): void {
    super.connectedCallback();

    // Tells the parent (`<ReactLitBridge />`) that there's a new
    // `ReactLitElement`, so the parent will render it.
    this.dispatchEvent(
      new CustomEvent<ReactLitElement>(ATTACHED_REACT_LIT_ELEMENT_EVENT, {
        detail: this,
        bubbles: true,
        composed: true,
      }),
    );
  }

  disconnectedCallback(): void {
    // Don't trigger update on the React side if we don't render the element
    // anymore.
    this.setState = null;
    // Tells the parent (`<ReactLitBridge />`) that this element is going
    // to be removed from DOM, so the parent will stop render it.
    this.dispatchEvent(
      new CustomEvent<ReactLitElement>(DETACHED_REACT_LIT_ELEMENT_EVENT, {
        detail: this,
        bubbles: true,
        composed: true,
      }),
    );
    super.disconnectedCallback();
  }

  // Use `readonly` to prevent child class from overriding this method.
  readonly render = () => {
    // Renders a slot so it will show the content from the light DOM child.
    return html`<slot></slot>`;
  };

  protected updated(
    // Use the same type definition as the super class.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    changedProperties: PropertyValueMap<any> | Map<PropertyKey, unknown>,
  ): void {
    super.update(changedProperties);
    // Trigger an update on the React side.
    this.setState?.({});
  }

  /**
   * Similar to `LitElement.render()` but uses React node instead of lit-html
   * template to define the output content.
   *
   * In the real DOM tree, the content is rendered as children of this node in
   * the light DOM.
   * In the React virtual DOM tree, the content is rendered as children of the
   * nearest ancestor `<ReactLitBridge />`.
   * The event and context propagation follows the propagation rules in their
   * respective DOM tree.
   */
  abstract renderReact(): ReactNode;

  readonly [USE_REACT_NODE] = () => {
    const [_, setState] = useState({});
    // Create a handle that can trigger update on the React side.
    // `setState` is referentially stable once its created.
    // It's guaranteed that the element can have at most one React component
    // instance at any given time.
    this.setState = setState;

    return this.renderReact();
  };
}

export interface ReactLitRendererProps {
  readonly element: ReactLitElement;
}

/**
 * Renders a ReactLitElement as a React component.
 */
export function ReactLitRenderer({ element }: ReactLitRendererProps) {
  const reactNode = element[USE_REACT_NODE]();
  return createPortal(reactNode, element);
}
