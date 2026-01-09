// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law-or-agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { DateTime } from 'luxon';

import { SmartRelativeTimestamp } from './smart_relative_timestamp';

describe('<SmartRelativeTimestamp />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date('2026-01-08T20:00:00Z'));
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('renders nothing if date is invalid', () => {
    const date = DateTime.fromISO('invalid-date');
    const { container } = render(<SmartRelativeTimestamp date={date} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('renders relative time correctly', () => {
    // 3 days ago
    const date = DateTime.fromISO('2026-01-05T20:00:00Z');
    render(<SmartRelativeTimestamp date={date} />);

    expect(screen.getByText('3 days ago')).toBeInTheDocument();
  });

  it('renders tooltip with local and UTC time on hover', async () => {
    render(
      <SmartRelativeTimestamp
        date={DateTime.fromISO('2026-01-05T20:00:00Z')}
      />,
    );

    const element = screen.getByText('3 days ago');
    fireEvent.mouseOver(element);

    await waitFor(async () => {
      expect(screen.getByText('Local:')).toBeInTheDocument();
      expect(screen.getByText('UTC:')).toBeInTheDocument();
      // Workaround DateTime.toLocaleString behavior, which may use a non-breaking
      // space.
      const tooltip = await screen.findByRole('tooltip');
      expect(tooltip).toHaveTextContent(/January 5, 2026 at 8:00.*PM UTC/);
    });
  });
});
