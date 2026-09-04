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

import { render, screen, fireEvent } from '@testing-library/react';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { SingleMetric } from './single_metric';

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
}));

describe('<SingleMetric />', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders metric name, value, total, and percentage', () => {
    render(
      <FakeContextProvider>
        <SingleMetric name="In Service" value={50} total={100} />
      </FakeContextProvider>,
    );

    expect(screen.getByText('In Service')).toBeInTheDocument();
    expect(screen.getByText('50')).toBeInTheDocument();
    expect(screen.getByText('/ 100')).toBeInTheDocument();
    expect(screen.getByText('50%')).toBeInTheDocument();
  });

  it('calls handleClick when clicked', () => {
    const handleClick = jest.fn();
    render(
      <FakeContextProvider>
        <SingleMetric name="In Service" value={50} handleClick={handleClick} />
      </FakeContextProvider>,
    );

    const button = screen.getByRole('button');
    fireEvent.click(button);
    expect(handleClick).toHaveBeenCalledTimes(1);
    expect(mockTrackEvent).toHaveBeenCalledWith('main_metric_clicked', {
      componentName: 'In Service',
    });
  });

  it('does not trigger handleClick or trackEvent when infoTooltip is clicked', () => {
    const handleClick = jest.fn();
    render(
      <FakeContextProvider>
        <SingleMetric
          name="In Service"
          value={50}
          handleClick={handleClick}
          infoTooltip={<span data-testid="tooltip-icon">Tooltip</span>}
        />
      </FakeContextProvider>,
    );

    const tooltipIcon = screen.getByTestId('tooltip-icon');
    fireEvent.click(tooltipIcon);
    expect(handleClick).not.toHaveBeenCalled();
    expect(mockTrackEvent).not.toHaveBeenCalled();

    fireEvent(
      tooltipIcon,
      new MouseEvent('auxclick', { bubbles: true, cancelable: true }),
    );
    expect(handleClick).not.toHaveBeenCalled();
    expect(mockTrackEvent).not.toHaveBeenCalled();

    fireEvent.keyDown(tooltipIcon, { key: 'Enter' });
    expect(handleClick).not.toHaveBeenCalled();
    expect(mockTrackEvent).not.toHaveBeenCalled();

    fireEvent.keyDown(tooltipIcon, { key: ' ' });
    expect(handleClick).not.toHaveBeenCalled();
    expect(mockTrackEvent).not.toHaveBeenCalled();
  });

  it('renders skeleton loaders when loading is true', () => {
    render(
      <FakeContextProvider>
        <SingleMetric name="In Service" value={50} loading={true} />
      </FakeContextProvider>,
    );

    const skeletons = screen.getAllByTestId('metric-skeleton');
    expect(skeletons.length).toBeGreaterThan(0);
  });
});
