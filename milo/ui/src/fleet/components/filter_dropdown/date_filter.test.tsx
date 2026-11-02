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
import { useState } from 'react';

import { DateFilterValue } from '@/fleet/types';

import { DateFilter } from './date_filter';

describe('DateFilter', () => {
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
});
