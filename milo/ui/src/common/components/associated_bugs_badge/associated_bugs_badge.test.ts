// Copyright 2022 The LUCI Authors.
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

import { fixture, oneEvent } from '@open-wc/testing-helpers';
import { html } from 'lit';

import './associated_bugs_badge';
import { AssociatedBugsTooltipElement } from '@/common/components/associated_bugs_tooltip';
import { ShowTooltipEventDetail } from '@/common/components/tooltip';
import { Cluster } from '@/common/services/luci_analysis';

import { AssociatedBugsBadgeElement } from './associated_bugs_badge';

const cluster1: Cluster = {
  clusterId: {
    algorithm: 'rule',
    id: 'cluster1',
  },
  bug: {
    system: 'monorail',
    id: '1234',
    linkText: 'crbug.com/1234',
    url: 'http://crbug.com/1234',
  },
};

const cluster2: Cluster = {
  clusterId: {
    algorithm: 'rule',
    id: 'cluster2',
  },
  bug: {
    system: 'monorail',
    id: '5678',
    linkText: 'crbug.com/5678',
    url: 'http://crbug.com/5678',
  },
};

const cluster3: Cluster = {
  clusterId: {
    algorithm: 'rule',
    id: 'cluster2',
  },
  bug: {
    system: 'buganizer',
    id: '1234',
    linkText: 'b/1234',
    url: 'http://b/1234',
  },
};

describe('AssociatedBugsBadge', () => {
  test('should remove duplicated bugs', async () => {
    const ele = await fixture<AssociatedBugsBadgeElement>(html`
      <milo-associated-bugs-badge
        .clusters=${[cluster1, cluster2, cluster3, cluster1]}
      ></milo-associated-bugs-badge>
    `);

    expect(ele.shadowRoot!.textContent).toContain('crbug.com/1234');
    expect(ele.shadowRoot!.textContent).not.toMatch(
      /crbug\.com\/1234(.*)crbug\.com\/1234/,
    );
  });

  test('should render a list on hover', async () => {
    const ele = await fixture<AssociatedBugsBadgeElement>(html`
      <milo-associated-bugs-badge
        .clusters=${[cluster1, cluster2, cluster3]}
      ></milo-associated-bugs-badge>
    `);

    const listener = oneEvent(window, 'show-tooltip');
    ele
      .shadowRoot!.querySelector('.badge')!
      .dispatchEvent(new MouseEvent('mouseover'));
    const event: CustomEvent<ShowTooltipEventDetail> = await listener;

    const tooltip = event.detail.tooltip.querySelector(
      'milo-associated-bugs-tooltip',
    )! as AssociatedBugsTooltipElement;

    expect(tooltip.clusters).toEqual([cluster1, cluster2, cluster3]);
  });
});
