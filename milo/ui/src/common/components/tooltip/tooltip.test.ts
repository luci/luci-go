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

import { expect } from '@jest/globals';
import { aTimeout, fixture, html } from '@open-wc/testing-helpers';

import './tooltip';
import {
  HideTooltipEventDetail,
  ShowTooltipEventDetail,
  TooltipElement,
} from './tooltip';

describe('Tooltip', () => {
  test('should only display one tooltip at a time', async () => {
    const tooltipContainer = await fixture<TooltipElement>(
      html`<milo-tooltip></milo-tooltip>`
    );

    const tooltip1 = document.createElement('div');
    const tooltip2 = document.createElement('div');

    window.dispatchEvent(
      new CustomEvent<ShowTooltipEventDetail>('show-tooltip', {
        detail: {
          tooltip: tooltip1,
          targetRect: tooltipContainer.getBoundingClientRect(),
          gapSize: 5,
        },
      })
    );

    await aTimeout(0);
    expect(tooltip1.isConnected).toBeTruthy();
    expect(tooltip2.isConnected).toBeFalsy();

    window.dispatchEvent(
      new CustomEvent<ShowTooltipEventDetail>('show-tooltip', {
        detail: {
          tooltip: tooltip2,
          targetRect: tooltipContainer.getBoundingClientRect(),
          gapSize: 5,
        },
      })
    );

    await aTimeout(0);
    expect(tooltip1.isConnected).toBeFalsy();
    expect(tooltip2.isConnected).toBeTruthy();
  });

  test('should hide tooltip after specified delay', async () => {
    const tooltipContainer = await fixture<TooltipElement>(
      html`<milo-tooltip></milo-tooltip>`
    );

    const tooltip = document.createElement('div');

    window.dispatchEvent(
      new CustomEvent<ShowTooltipEventDetail>('show-tooltip', {
        detail: {
          tooltip,
          targetRect: tooltipContainer.getBoundingClientRect(),
          gapSize: 5,
        },
      })
    );

    await aTimeout(0);
    expect(tooltip.isConnected).toBeTruthy();

    window.dispatchEvent(
      new CustomEvent<HideTooltipEventDetail>('hide-tooltip', {
        detail: { delay: 10 },
      })
    );

    await aTimeout(5);
    expect(tooltip.isConnected).toBeTruthy();

    await aTimeout(5);
    expect(tooltip.isConnected).toBeFalsy();
  });

  test('should handle race condition correctly', async () => {
    const tooltipContainer = await fixture<TooltipElement>(
      html`<milo-tooltip></milo-tooltip>`
    );

    const tooltip1 = document.createElement('div');
    const tooltip2 = document.createElement('div');

    window.dispatchEvent(
      new CustomEvent<ShowTooltipEventDetail>('show-tooltip', {
        detail: {
          tooltip: tooltip1,
          targetRect: tooltipContainer.getBoundingClientRect(),
          gapSize: 5,
        },
      })
    );

    await aTimeout(0);
    expect(tooltip1.isConnected).toBeTruthy();

    window.dispatchEvent(
      new CustomEvent<HideTooltipEventDetail>('hide-tooltip', {
        detail: { delay: 10 },
      })
    );

    await aTimeout(0);
    expect(tooltip1.isConnected).toBeTruthy();

    // Show another tooltip before the first one is dismissed.
    window.dispatchEvent(
      new CustomEvent<ShowTooltipEventDetail>('show-tooltip', {
        detail: {
          tooltip: tooltip2,
          targetRect: tooltipContainer.getBoundingClientRect(),
          gapSize: 5,
        },
      })
    );

    await aTimeout(10);
    expect(tooltip1.isConnected).toBeFalsy();
    expect(tooltip2.isConnected).toBeTruthy();
  });
});
