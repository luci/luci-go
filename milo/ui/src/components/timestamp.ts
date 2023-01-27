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

import { MobxLitElement } from '@adobe/lit-mobx';
import { css, html, render } from 'lit';
import { customElement } from 'lit/decorators.js';
import { DateTime } from 'luxon';
import { makeObservable, observable } from 'mobx';

import { displayDuration, LONG_TIME_FORMAT } from '../libs/time_utils';
import { HideTooltipEventDetail, ShowTooltipEventDetail } from './tooltip';

export interface TimeZoneConfig {
  label: string;
  zone: string;
}

export const DEFAULT_EXTRA_ZONE_CONFIGS = [
  {
    label: 'PT',
    zone: 'America/Los_Angeles',
  },
  {
    label: 'UTC',
    zone: 'utc',
  },
];

/**
 * Renders a timestamp.
 * Shows duration and addition timezone on hover.
 */
@customElement('milo-timestamp')
export class TimestampElement extends MobxLitElement {
  @observable.ref datetime!: DateTime;
  @observable.ref format = LONG_TIME_FORMAT;
  @observable.ref extraZones = DEFAULT_EXTRA_ZONE_CONFIGS;

  constructor() {
    super();
    makeObservable(this);
  }

  private renderTooltip() {
    const now = DateTime.now();
    return html`
      <table>
        <tr>
          <td colspan="2">${displayDuration(now.diff(this.datetime))} ago</td>
        </tr>
        ${this.extraZones.map(
          (tz) => html`
            <tr>
              <td>${tz.label}:</td>
              <td>${this.datetime.setZone(tz.zone).toFormat(this.format)}</td>
            </tr>
          `
        )}
      </table>
    `;
  }

  protected render() {
    return html`
      <span
        @mouseover=${(e: MouseEvent) => {
          const tooltip = document.createElement('div');
          render(this.renderTooltip(), tooltip);

          window.dispatchEvent(
            new CustomEvent<ShowTooltipEventDetail>('show-tooltip', {
              detail: {
                tooltip,
                targetRect: (e.target as HTMLElement).getBoundingClientRect(),
                gapSize: 2,
              },
            })
          );
        }}
        @mouseout=${() => {
          window.dispatchEvent(new CustomEvent<HideTooltipEventDetail>('hide-tooltip', { detail: { delay: 50 } }));
        }}
      >
        ${this.datetime.toFormat(this.format)}
      </span>
    `;
  }

  static styles = css`
    :host {
      display: inline;
    }
    a {
      color: var(--default-text-color);
    }
  `;
}
