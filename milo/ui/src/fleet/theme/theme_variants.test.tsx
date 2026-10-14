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

import { Button, Card, Chip } from '@mui/material';
import { screen } from '@testing-library/react';

import { renderWithFleetTheme } from '@/fleet/testing_tools/test_provider';
import {
  borders,
  controlHeights,
  fleetColors,
  radii,
} from '@/fleet/theme/tokens';

describe('Fleet GM3 Theme Variants', () => {
  it('renders Button variant="toolbar" as a 32px pill with primary styling', () => {
    renderWithFleetTheme(<Button variant="toolbar">Deploy</Button>);
    const button = screen.getByRole('button', { name: 'Deploy' });
    expect(button).toBeInTheDocument();
    expect(button).toHaveStyle({
      borderRadius: radii.pill,
      height: `${controlHeights.button}px`,
    });
  });

  it('renders Chip variant="filter" with primary container background', () => {
    renderWithFleetTheme(
      <Chip variant="filter" label="Dut State: Available" />,
    );
    const chip = screen.getByText('Dut State: Available');
    expect(chip).toBeInTheDocument();
  });

  it('renders Card variant="surface" with 12px radius and high-contrast border', () => {
    renderWithFleetTheme(
      <Card variant="surface" data-testid="surface-card">
        Surface Card Content
      </Card>,
    );
    const card = screen.getByTestId('surface-card');
    expect(card).toBeInTheDocument();
    expect(card).toHaveStyle({
      backgroundColor: fleetColors.surface,
      border: borders.default,
      borderRadius: radii.md,
    });
  });
});
