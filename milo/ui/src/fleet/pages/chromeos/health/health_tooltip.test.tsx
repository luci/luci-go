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

import { HealthTooltip } from './health_tooltip';

describe('HealthTooltip', () => {
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
});
