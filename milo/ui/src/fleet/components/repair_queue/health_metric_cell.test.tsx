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

import { fireEvent, render, screen } from '@testing-library/react';

import { HealthPairCell } from './health_metric_cell';

describe('HealthPairCell', () => {
  it('renders plain percentages and dashes when no in-flight repairs exist', () => {
    const { rerender } = render(
      <HealthPairCell
        primaryLabel="Host Group"
        primaryHealthPct={0.75}
        secondaryLabel="Run Target"
        secondaryHealthPct={0.5}
      />,
    );

    expect(screen.getByText('75% / 50%')).toBeInTheDocument();
    expect(
      screen.queryByTestId('host-group-in-repair-badge'),
    ).not.toBeInTheDocument();

    rerender(
      <HealthPairCell
        primaryLabel="Host Group"
        primaryHealthPct={0.6}
        secondaryLabel="Run Target"
      />,
    );
    expect(screen.getByText('60% / -')).toBeInTheDocument();
  });

  it('renders (+X%) badges and breakdown tooltip when in-flight repairs are active', async () => {
    render(
      <HealthPairCell
        primaryLabel="Host Group"
        primaryHealthPct={0.6}
        primaryInRepairPct={0.2}
        secondaryLabel="Run Target"
        secondaryHealthPct={0.5}
        secondaryInRepairPct={0.25}
      />,
    );

    const hgBadge = screen.getByTestId('host-group-in-repair-badge');
    const rtBadge = screen.getByTestId('run-target-in-repair-badge');
    expect(hgBadge).toHaveTextContent('(+20%)');
    expect(rtBadge).toHaveTextContent('(+25%)');

    fireEvent.mouseOver(hgBadge);
    expect(
      await screen.findByText('Host Group Health Breakdown'),
    ).toBeInTheDocument();
    expect(screen.getByText('+20%')).toBeInTheDocument();
    expect(screen.getByText('80%')).toBeInTheDocument();
  });
});
