// Copyright 2026 The LUCI Authors.
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

import { shouldIgnoreClickAway } from './click_away';

describe('shouldIgnoreClickAway', () => {
  it('returns true for a detached/unmounted node originating inside a popper/menu hierarchy', () => {
    const detachedEl = document.createElement('div');
    const popperEl = document.createElement('div');
    popperEl.classList.add('MuiPopper-root');

    const event = new MouseEvent('click');
    Object.defineProperty(event, 'target', { value: detachedEl });
    Object.defineProperty(event, 'composedPath', {
      value: () => [detachedEl, popperEl, document.body, window],
    });

    expect(shouldIgnoreClickAway(event, null)).toBe(true);
  });

  it('returns false for an external detached node outside any popper or menu hierarchy (allows closing)', () => {
    const detachedEl = document.createElement('button');
    const event = new MouseEvent('click');
    Object.defineProperty(event, 'target', { value: detachedEl });
    Object.defineProperty(event, 'composedPath', {
      value: () => [detachedEl, document.body, window],
    });

    expect(shouldIgnoreClickAway(event, null)).toBe(false);
  });

  it('returns true when target is inside anchorEl', () => {
    const anchorEl = document.createElement('div');
    const childEl = document.createElement('button');
    anchorEl.appendChild(childEl);
    document.body.appendChild(anchorEl);

    const event = new MouseEvent('click');
    Object.defineProperty(event, 'target', { value: childEl });

    expect(shouldIgnoreClickAway(event, anchorEl)).toBe(true);

    document.body.removeChild(anchorEl);
  });

  it('returns false when target is connected and outside anchorEl', () => {
    const anchorEl = document.createElement('div');
    const outsideEl = document.createElement('div');
    document.body.appendChild(anchorEl);
    document.body.appendChild(outsideEl);

    const event = new MouseEvent('click');
    Object.defineProperty(event, 'target', { value: outsideEl });

    expect(shouldIgnoreClickAway(event, anchorEl)).toBe(false);

    document.body.removeChild(anchorEl);
    document.body.removeChild(outsideEl);
  });

  it('returns false when event.target is null or undefined', () => {
    const event = new MouseEvent('click');
    Object.defineProperty(event, 'target', { value: null });
    expect(shouldIgnoreClickAway(event, document.createElement('div'))).toBe(
      false,
    );
  });

  it('returns false when target is connected and anchorEl/containerEl are null or undefined', () => {
    const connectedEl = document.createElement('div');
    document.body.appendChild(connectedEl);
    const event = new MouseEvent('click');
    Object.defineProperty(event, 'target', { value: connectedEl });

    expect(shouldIgnoreClickAway(event, null)).toBe(false);
    expect(shouldIgnoreClickAway(event, undefined)).toBe(false);
    document.body.removeChild(connectedEl);
  });

  it('safely handles virtual anchors or objects without a valid contains() method', () => {
    const connectedEl = document.createElement('div');
    document.body.appendChild(connectedEl);
    const event = new MouseEvent('click');
    Object.defineProperty(event, 'target', { value: connectedEl });

    const virtualAnchor = {
      getBoundingClientRect: () => ({ top: 0, left: 0, width: 0, height: 0 }),
    } as unknown as Element;
    expect(shouldIgnoreClickAway(event, virtualAnchor)).toBe(false);
    document.body.removeChild(connectedEl);
  });

  it('returns true when containerEl is in composedPath even if target is detached', () => {
    const containerEl = document.createElement('div');
    const detachedButton = document.createElement('button');
    const event = new MouseEvent('click');
    Object.defineProperty(event, 'target', { value: detachedButton });
    Object.defineProperty(event, 'composedPath', {
      value: () => [detachedButton, containerEl, document.body, window],
    });

    expect(shouldIgnoreClickAway(event, null, containerEl)).toBe(true);
  });

  it('handles SVGElement and Text nodes correctly inside anchorEl', () => {
    const anchorEl = document.createElement('div');
    const svgEl = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    const textNode = document.createTextNode('sample text');
    anchorEl.appendChild(svgEl);
    anchorEl.appendChild(textNode);
    document.body.appendChild(anchorEl);

    const svgEvent = new MouseEvent('click');
    Object.defineProperty(svgEvent, 'target', { value: svgEl });
    expect(shouldIgnoreClickAway(svgEvent, anchorEl)).toBe(true);

    const textEvent = new MouseEvent('click');
    Object.defineProperty(textEvent, 'target', { value: textNode });
    expect(shouldIgnoreClickAway(textEvent, anchorEl)).toBe(true);

    document.body.removeChild(anchorEl);
  });
});
