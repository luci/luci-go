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

import { fireEvent, screen } from '@testing-library/react';

import { renderWithFleetTheme } from '@/fleet/testing_tools/test_provider';
import { radii } from '@/fleet/theme/tokens';

import { SmallMetricItem } from './small_metric_item';

describe('<SmallMetricItem />', () => {
  it('renders label, value, and percentage with 6px radius and accessible label', () => {
    const handleClick = jest.fn();
    renderWithFleetTheme(
      <SmallMetricItem
        label="Ready:"
        value={42}
        total={100}
        onClick={handleClick}
      />,
    );

    const button = screen.getByRole('button', { name: 'Ready' });
    expect(button).toBeInTheDocument();
    expect(button).toHaveStyle({
      borderRadius: radii.sm,
    });
    expect(screen.getByText('42')).toBeInTheDocument();
    expect(screen.getByText('(42.0%)')).toBeInTheDocument();

    fireEvent.click(button);
    expect(handleClick).toHaveBeenCalledTimes(1);
  });
});
