// Copyright 2024 The LUCI Authors.
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

import { render, screen } from '@testing-library/react';

import { StatusBar } from './status_bar';

describe('StatusBar', () => {
  it('should render components correctly', () => {
    const components = [
      { segmentColor: 'red', weight: 1 },
      { segmentColor: 'blue', weight: 2 },
    ];
    render(<StatusBar components={components} />);

    const segments = screen.getAllByTestId('status-bar-segment');
    screen.debug(segments[0]);
    expect(segments).toHaveLength(2);
    expect(segments[0]).toHaveAttribute(
      'style',
      expect.stringContaining('background-color: red'),
    );
    expect(segments[1]).toHaveAttribute(
      'style',
      expect.stringContaining('background-color: blue'),
    );
  });

  it('should render loading state', () => {
    render(<StatusBar components={[]} loading={true} />);
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });
});
