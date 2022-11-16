// Copyright 2021 The LUCI Authors.
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

import { customElement, LitElement } from 'lit-element';
import { html } from 'lit-html';

export interface DragEventDetails {
  // relative to the start.
  dx: number;
  dy: number;
}

export type DragEvent = CustomEvent<DragEventDetails>;

/**
 * A element that tracks the dragging event.
 * Comparing to the native HTML drag event, this has higher resolution, and
 * fires dragend event consistently when dragging stopped.
 */
@customElement('milo-drag-tracker')
export class DragTrackerElement extends LitElement {
  constructor() {
    super();

    this.addEventListener('mousedown', (startEvent: MouseEvent) => {
      const x = startEvent.pageX;
      const y = startEvent.pageY;

      const dispatchEvent = (e: MouseEvent, type: string) => {
        const dx = e.pageX - x;
        const dy = e.pageY - y;
        this.dispatchEvent(new CustomEvent<DragEventDetails>(type, { detail: { dx, dy } }));
      };

      const onMouseMove = (dragEvent: MouseEvent) => {
        dragEvent.preventDefault();
        dispatchEvent(dragEvent, 'drag');
      };
      document.addEventListener('mousemove', onMouseMove);

      document.addEventListener(
        'mouseup',
        (endEvent: MouseEvent) => {
          dispatchEvent(endEvent, 'dragend');
          document.removeEventListener('mousemove', onMouseMove);
        },
        { once: true }
      );

      dispatchEvent(startEvent, 'dragstart');
    });
  }

  protected render() {
    return html``;
  }
}
