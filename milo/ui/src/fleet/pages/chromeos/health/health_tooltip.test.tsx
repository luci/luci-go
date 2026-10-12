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

import {
  HealthThresholdsTooltip,
  HealthThresholdsTooltipContent,
  HealthTooltip,
} from './health_tooltip';

describe('HealthThresholdsTooltipContent', () => {
  it('renders header and all three SLA threshold rows with labels and ranges', () => {
    render(<HealthThresholdsTooltipContent />);

    expect(screen.getByText('Health Thresholds')).toBeInTheDocument();

    const healthyRow = screen.getByTestId('threshold-row-healthy');
    expect(healthyRow).toHaveTextContent('Healthy');
    expect(healthyRow).toHaveTextContent('≥ 95%');

    const warningRow = screen.getByTestId('threshold-row-warning');
    expect(warningRow).toHaveTextContent('Warning');
    expect(warningRow).toHaveTextContent('80%–94%');

    const criticalRow = screen.getByTestId('threshold-row-critical');
    expect(criticalRow).toHaveTextContent('Critical');
    expect(criticalRow).toHaveTextContent('< 80%');
  });

  it('renders current value header and highlights active tier when pct is provided', () => {
    render(<HealthThresholdsTooltipContent pct={75} metricLabel="Health" />);

    expect(screen.getByTestId('threshold-current-value')).toHaveTextContent(
      'Current Health:75% (Critical)',
    );

    const criticalRow = screen.getByTestId('threshold-row-critical');
    expect(criticalRow).toHaveTextContent('Critical (Current)');
    expect(criticalRow).toHaveTextContent('< 80%');
  });
});

describe('HealthTooltip and HealthThresholdsTooltip components', () => {
  it('renders HealthTooltip on hover with custom title', async () => {
    render(
      <HealthTooltip title="Custom health explanation">
        <button type="button">Hover me</button>
      </HealthTooltip>,
    );

    fireEvent.mouseOver(screen.getByText('Hover me'));
    expect(
      await screen.findByText('Custom health explanation'),
    ).toBeInTheDocument();
  });

  it('renders HealthThresholdsTooltip with thresholds content on hover', async () => {
    render(
      <HealthThresholdsTooltip pct={96} metricLabel="Availability">
        <button type="button">Target button</button>
      </HealthThresholdsTooltip>,
    );

    fireEvent.mouseOver(screen.getByText('Target button'));
    expect(await screen.findByText('Health Thresholds')).toBeInTheDocument();
    expect(screen.getByTestId('threshold-current-value')).toHaveTextContent(
      'Current Availability:96% (Healthy)',
    );
  });
});
