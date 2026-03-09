// Copyright 2025 The LUCI Authors.
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

import { RangeFilter } from '@/fleet/components/filter_dropdown/range_filter';

describe('RangeFilter', () => {
  it('should correctly allow missing boundaries (emptying input) without coercing back to min/max', () => {
    const mockOnChange = jest.fn();

    render(
      <RangeFilter
        min={0}
        max={100}
        value={{ min: 0, max: 100 }}
        onChange={mockOnChange}
      />,
    );

    const minInput = screen.getByLabelText('Min') as HTMLInputElement;

    // Simulate clearing the input field
    fireEvent.change(minInput, { target: { value: '' } });

    expect(minInput.value).toBe('');
    expect(mockOnChange).toHaveBeenCalledWith({ min: undefined, max: 100 });
  });

  it('should not coercively evaluate an empty string to NaN visually', () => {
    const mockOnChange = jest.fn();

    render(
      <RangeFilter
        min={-5}
        max={15}
        value={{ min: undefined, max: 15 }}
        onChange={mockOnChange}
      />,
    );

    const minInput = screen.getByLabelText('Min') as HTMLInputElement;

    // Value should be empty, not 'NaN'
    expect(minInput.value).toBe('');
  });
});
