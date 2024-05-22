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
  OPEN_PORTAL_EVENT,
  ReactLitElement,
  CLOSE_PORTAL_EVENT,
} from './react_lit_element';
import { ReactLitRenderer } from './renderer';

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
  const portalsRef = useRef<Map<ReactLitElement, number>>();
  const trackerRef = useRef<ReactLitElementTrackerElement>();
  useEffect(() => {
    const tracker = trackerRef.current!;

    if (portalsRef.current === undefined) {
      // When `portalsRef` is initialized and there are some registered portals
      // already, trigger a rerender immediately.
      if (tracker[PORTALS].size !== 0) {
        setState({});
      }
    }
    portalsRef.current = tracker[PORTALS];

    // Rerender whenever there's an update on the portals set.
    function onPortalsUpdated(e: Event) {
      e.stopPropagation();
      setState({});
    }
    tracker.addEventListener(PORTALS_UPDATED_EVENT, onPortalsUpdated);
    return () =>
      tracker.removeEventListener(PORTALS_UPDATED_EVENT, onPortalsUpdated);
  }, []);

  return (
    // In Lit, the portal open event is fired during connection phase. In React,
    // we can only add custom event handler to an element after it's rendered
    // (and connected), therefore the custom event handler may miss portals
    // that are rendered in the same rendering cycle as `<ReactLitBridge />`.
    //
    // Custom web component can attach event handlers during connection phase.
    // Use a web component to collect all the portals so we don't miss any
    // event fired during the connection phase.
    <react-lit-element-tracker ref={trackerRef}>
      {children}
      {[...(portalsRef.current?.entries() || [])].map(([portal, id]) => (
        <ReactLitRenderer key={id} portal={portal} />
      ))}
    </react-lit-element-tracker>
  );
}

const PORTALS_UPDATED_EVENT = 'portals-updated';

// Use a symbol to ensure the property cannot be accessed anywhere else.
const PORTALS = Symbol('portals');

@customElement('react-lit-element-tracker')
class ReactLitElementTrackerElement extends LitElement {
  private nextId = 0;
  readonly [PORTALS] = new Map<ReactLitElement, number>();

  private onOpenPortal = (e: Event) => {
    const event = e as CustomEvent<ReactLitElement>;
    if (this[PORTALS].has(event.detail)) {
      return;
    }
    this[PORTALS].set(event.detail, this.nextId);
    this.nextId++;
    this.dispatchEvent(new CustomEvent(PORTALS_UPDATED_EVENT));
  };

  private onClosePortal = (e: Event) => {
    const event = e as CustomEvent<ReactLitElement>;
    if (!this[PORTALS].has(event.detail)) {
      return;
    }
    this[PORTALS].delete(event.detail);
    this.dispatchEvent(new CustomEvent(PORTALS_UPDATED_EVENT));
  };

  connectedCallback(): void {
    super.connectedCallback();
    this.addEventListener(OPEN_PORTAL_EVENT, this.onOpenPortal);
    this.addEventListener(CLOSE_PORTAL_EVENT, this.onClosePortal);
  }

  disconnectedCallback(): void {
    this.removeEventListener(CLOSE_PORTAL_EVENT, this.onClosePortal);
    this.removeEventListener(OPEN_PORTAL_EVENT, this.onOpenPortal);

    if (this[PORTALS].size !== 0) {
      this[PORTALS].clear();
      this.dispatchEvent(new CustomEvent(PORTALS_UPDATED_EVENT));
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
