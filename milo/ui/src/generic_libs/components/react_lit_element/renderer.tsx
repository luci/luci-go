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

import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

import { ELEMENT_UPDATED_EVENT, ReactLitElement } from './react_lit_element';

export interface ReactLitRendererProps {
  readonly portal: ReactLitElement;
}

/**
 * Renders a ReactLitElement as a React component.
 */
export function ReactLitRenderer({ portal }: ReactLitRendererProps) {
  const [_, setState] = useState({});

  // Portal's state is tracked by lit-element state management.
  // When we detect an update from lit-element, trigger a React rerender.
  const onElementUpdatedRef = useRef(() => setState({}));

  // Subscribe immediately before rendering when a new portal is received so we
  // don't miss any update events.
  const portalRef = useRef<ReactLitElement | null>(null);
  if (portalRef.current !== portal) {
    portal.addEventListener(ELEMENT_UPDATED_EVENT, onElementUpdatedRef.current);
    portalRef.current = portal;
  }

  // Clean up the event handler.
  useEffect(() => {
    const onElementUpdated = onElementUpdatedRef.current;
    return () =>
      portal.removeEventListener(ELEMENT_UPDATED_EVENT, onElementUpdated);
  }, [portal]);

  return createPortal(portal.renderReact(), portal);
}
