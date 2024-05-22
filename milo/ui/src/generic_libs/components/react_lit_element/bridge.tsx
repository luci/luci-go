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
import {
  MutableRefObject,
  ReactNode,
  useEffect,
  useRef,
  useState,
} from 'react';

import {
  ATTACHED_REACT_LIT_ELEMENT_EVENT,
  ReactLitElement,
  DETACHED_REACT_LIT_ELEMENT_EVENT,
  ReactLitRenderer,
} from './react_lit_element';

export interface ReactLitBridgeProps {
  readonly children: ReactNode;
}

/**
 * Enables the use of `ReactLitElement` based custom web components.
 *
 * React context available to this component will also be available to all
 * `ReactLitElement` descendants.
 */
export function ReactLitBridge({ children }: ReactLitBridgeProps) {
  const [_, setState] = useState({});
  const elementsRef = useRef<Map<ReactLitElement, number>>();
  const trackerRef = useRef<ReactLitElementTrackerElement>();
  useEffect(() => {
    const tracker = trackerRef.current!;

    if (elementsRef.current === undefined) {
      // When `elementsRef` is initialized and there are some registered
      // elements already, trigger a rerender immediately.
      if (tracker[ELEMENTS].size !== 0) {
        setState({});
      }
    }
    elementsRef.current = tracker[ELEMENTS];

    // Rerender whenever there's an update on the element set.
    function onElementSetUpdated(e: Event) {
      e.stopPropagation();
      setState({});
    }
    tracker.addEventListener(ELEMENT_SET_UPDATED_EVENT, onElementSetUpdated);
    return () =>
      tracker.removeEventListener(
        ELEMENT_SET_UPDATED_EVENT,
        onElementSetUpdated,
      );
  }, []);

  return (
    // In Lit, the element created event is fired during connection phase. In
    // React, we can only add custom event handler to an element after it's
    // rendered (and connected), therefore the custom event handler may miss
    // elements that are created in the same rendering cycle as
    // `<ReactLitBridge />`.
    //
    // Custom web component can attach event handlers during connection phase.
    // Use a web component so we don't miss any element created during the
    // connection phase.
    <react-lit-element-tracker ref={trackerRef}>
      {children}
      {[...(elementsRef.current?.entries() || [])].map(([ele, id]) => (
        <ReactLitRenderer key={id} element={ele} />
      ))}
    </react-lit-element-tracker>
  );
}

const ELEMENT_SET_UPDATED_EVENT = 'element-set-updated';

// Use a symbol to ensure the property cannot be accessed anywhere else.
const ELEMENTS = Symbol('elements');

@customElement('react-lit-element-tracker')
class ReactLitElementTrackerElement extends LitElement {
  private nextId = 0;
  readonly [ELEMENTS] = new Map<ReactLitElement, number>();

  private onAttachedElement = (e: Event) => {
    const event = e as CustomEvent<ReactLitElement>;
    if (this[ELEMENTS].has(event.detail)) {
      return;
    }
    this[ELEMENTS].set(event.detail, this.nextId);
    this.nextId++;
    this.dispatchEvent(new CustomEvent(ELEMENT_SET_UPDATED_EVENT));
  };

  private onDetachedElement = (e: Event) => {
    const event = e as CustomEvent<ReactLitElement>;
    if (!this[ELEMENTS].has(event.detail)) {
      return;
    }
    this[ELEMENTS].delete(event.detail);
    this.dispatchEvent(new CustomEvent(ELEMENT_SET_UPDATED_EVENT));
  };

  connectedCallback(): void {
    super.connectedCallback();
    this.addEventListener(
      ATTACHED_REACT_LIT_ELEMENT_EVENT,
      this.onAttachedElement,
    );
    this.addEventListener(
      DETACHED_REACT_LIT_ELEMENT_EVENT,
      this.onDetachedElement,
    );
  }

  disconnectedCallback(): void {
    this.removeEventListener(
      ATTACHED_REACT_LIT_ELEMENT_EVENT,
      this.onDetachedElement,
    );
    this.removeEventListener(
      DETACHED_REACT_LIT_ELEMENT_EVENT,
      this.onAttachedElement,
    );

    if (this[ELEMENTS].size !== 0) {
      this[ELEMENTS].clear();
      this.dispatchEvent(new CustomEvent(ELEMENT_SET_UPDATED_EVENT));
    }

    super.connectedCallback();
  }

  protected render() {
    return html`<slot></slot>`;
  }
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'react-lit-element-tracker': {
        readonly ref: MutableRefObject<
          ReactLitElementTrackerElement | undefined
        >;
        readonly children: ReactNode;
      };
    }
  }
}
