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

import { fireEvent, render, screen, within } from '@testing-library/react';
import { DateTime } from 'luxon';
import { useState } from 'react';

import { DateFilterValue } from '@/fleet/types';

import { DateFilter } from './date_filter';

describe('DateFilter', () => {
  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  it('keeps the calendar popper positioned when selecting time slots so focus does not scroll the page', () => {
    function StatefulDateFilter() {
      const [val, setVal] = useState<DateFilterValue>({
        min: new Date('2026-10-31T05:00:00.000Z'),
      });
      return <DateFilter value={val} onChange={setVal} />;
    }

    render(<StatefulDateFilter />);

    fireEvent.click(screen.getAllByTestId('CalendarIcon')[0]);

    const popperEl = document.querySelector<HTMLElement>(
      '.MuiPickersPopper-root',
    );
    expect(popperEl).not.toBeNull();

    const popperPositionsDuringFocus: string[] = [];
    const origFocus = HTMLElement.prototype.focus;
    const focusSpy = jest
      .spyOn(HTMLElement.prototype, 'focus')
      .mockImplementation(function (this: HTMLElement, options?: FocusOptions) {
        if (
          this.classList.contains('MuiMultiSectionDigitalClockSection-item')
        ) {
          popperPositionsDuringFocus.push(popperEl?.style.position ?? '');
        }
        return origFocus.call(this, options);
      });

    try {
      fireEvent.click(screen.getByRole('option', { name: '8 hours' }));
      expect(popperPositionsDuringFocus.length).toBeGreaterThan(0);
      expect(popperPositionsDuringFocus).not.toContain('');
    } finally {
      focusSpy.mockRestore();
    }
  });

  it('preserves existing min and max date bounds and clears onlyEmpty when toggling Exclude (Blank)', () => {
    const min = new Date('2026-01-01T00:00:00.000Z');
    const max = new Date('2026-01-02T00:00:00.000Z');
    const handleChange = jest.fn();

    render(
      <DateFilter
        value={{
          min,
          max,
          onlyEmpty: true,
        }}
        onChange={handleChange}
      />,
    );

    const excludeCheckbox = screen.getByRole('checkbox', {
      name: 'Exclude (Blank)',
    });
    const excludeMenuItem = excludeCheckbox.closest('li');
    expect(excludeMenuItem).not.toBeNull();
    expect(
      within(excludeMenuItem!).queryByText('Only'),
    ).not.toBeInTheDocument();

    fireEvent.click(excludeCheckbox);

    expect(handleChange).toHaveBeenCalledWith({
      min,
      max,
      excludeEmpty: true,
      onlyEmpty: undefined,
    });
  });

  it('preserves existing min and max date bounds and clears excludeEmpty when toggling Include (Blank)', () => {
    const min = new Date('2026-01-01T00:00:00.000Z');
    const max = new Date('2026-01-02T00:00:00.000Z');
    const handleChange = jest.fn();

    render(
      <DateFilter
        value={{
          min,
          max,
          excludeEmpty: true,
        }}
        onChange={handleChange}
      />,
    );

    const includeCheckbox = screen.getByRole('checkbox', {
      name: 'Include (Blank)',
    });
    const includeMenuItem = includeCheckbox.closest('li');
    expect(includeMenuItem).not.toBeNull();
    expect(within(includeMenuItem!).getByText('Only')).toBeInTheDocument();

    fireEvent.click(includeCheckbox);

    expect(handleChange).toHaveBeenCalledWith({
      min,
      max,
      onlyEmpty: true,
      excludeEmpty: undefined,
    });
  });

  it('recomputes activePresetId when value prop updates to a preset timestamp', () => {
    jest.useFakeTimers();
    try {
      jest.setSystemTime(new Date('2026-10-22T12:00:00.000Z'));
      const fifteenMinAgo = DateTime.fromISO('2026-10-22T11:45:00.000Z', {
        zone: 'utc',
      }).toJSDate();
      const handleChange = jest.fn();
      const { rerender } = render(
        <DateFilter value={{}} onChange={handleChange} />,
      );

      rerender(
        <DateFilter value={{ min: fifteenMinAgo }} onChange={handleChange} />,
      );
      const chip = screen.getByRole('button', { name: 'Last 15m' });
      expect(chip).toHaveAttribute('aria-pressed', 'true');
      fireEvent.click(chip);
      expect(handleChange).toHaveBeenCalledWith({});
    } finally {
      jest.useRealTimers();
    }
  });

  it('clears the selected preset chip when From or To is manually edited within the preset tolerance window', () => {
    const fixedNow = DateTime.fromISO('2026-10-22T12:00:00.000Z', {
      zone: 'utc',
    }) as DateTime<true>;
    jest.spyOn(DateTime, 'now').mockReturnValue(fixedNow);

    function StatefulDateFilter() {
      const [val, setVal] = useState<DateFilterValue>({});
      return <DateFilter value={val} onChange={setVal} />;
    }

    render(<StatefulDateFilter />);

    const last15mChip = screen.getByRole('button', { name: 'Last 15m' });
    fireEvent.click(last15mChip);
    expect(last15mChip).toHaveAttribute('aria-pressed', 'true');

    // Manually edit From via the clock picker to 11:40 (20m ago, within the 5m tolerance window).
    fireEvent.click(screen.getAllByTestId('CalendarIcon')[0]);
    fireEvent.click(screen.getByRole('option', { name: '40 minutes' }));
    expect(last15mChip).toHaveAttribute('aria-pressed', 'false');

    // Clicking Last 15m again re-applies the preset rather than clearing the filter.
    fireEvent.click(last15mChip);
    expect(last15mChip).toHaveAttribute('aria-pressed', 'true');

    // Select a To preset (> 30m ago -> 11:30) and manually edit To to 11:25 (35m ago, within tolerance).
    const older30mChip = screen.getByRole('button', { name: '> 30m ago' });
    fireEvent.click(older30mChip);
    expect(older30mChip).toHaveAttribute('aria-pressed', 'true');

    fireEvent.click(screen.getAllByTestId('CalendarIcon')[1]);
    const toMinuteOptions = screen.getAllByRole('option', {
      name: '25 minutes',
    });
    fireEvent.click(toMinuteOptions[toMinuteOptions.length - 1]);
    expect(older30mChip).toHaveAttribute('aria-pressed', 'false');
  });
});
