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

import { render, screen, fireEvent, waitFor } from '@testing-library/react';

import { BuganizerLink } from './buganizer_link';

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
}));

describe('BuganizerLink', () => {
  test('should render the link to Buganizer', () => {
    render(<BuganizerLink name="test-device" />);

    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', 'http://b/test-device');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noreferrer');
  });

  test('should render the link with multiple names merged with OR', () => {
    render(<BuganizerLink name={['device1', 'device2']} />);

    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', 'http://b/device1%20OR%20device2');
  });

  test('should have the correct tooltip', async () => {
    render(<BuganizerLink name="test-device" />);

    const link = screen.getByRole('link');
    fireEvent.mouseOver(link);

    await waitFor(async () => {
      const tooltip = await screen.findByRole('tooltip');
      expect(tooltip).toHaveTextContent(
        'Search for bugs related to this device in Buganizer',
      );
    });
  });

  test('should track event when clicked', () => {
    render(<BuganizerLink name="test-device" />);

    const link = screen.getByRole('link');
    fireEvent.click(link);

    expect(mockTrackEvent).toHaveBeenCalledWith('buganizer_link_dut_clicked', {
      componentName: 'BuganizerLink',
    });
  });

  test('should track event with project when clicked', () => {
    render(<BuganizerLink name="test-device" project="test-project" />);

    const link = screen.getByRole('link');
    fireEvent.click(link);

    expect(mockTrackEvent).toHaveBeenCalledWith('buganizer_link_dut_clicked', {
      componentName: 'BuganizerLink',
      project: 'test-project',
    });
  });
});
