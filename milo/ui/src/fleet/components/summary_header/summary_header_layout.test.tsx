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

import { render, screen } from '@testing-library/react';

import { SubMetricList, SummaryHeaderTitle } from './summary_header_layout';

describe('summary_header_layout', () => {
  it('renders SummaryHeaderTitle with h5 heading', () => {
    render(<SummaryHeaderTitle>Test Header Title</SummaryHeaderTitle>);
    const heading = screen.getByRole('heading', {
      level: 5,
      name: 'Test Header Title',
    });
    expect(heading).toBeInTheDocument();
  });

  it('renders SubMetricList children in a container', () => {
    render(
      <SubMetricList>
        <div>Child Metric Item</div>
      </SubMetricList>,
    );
    expect(screen.getByText('Child Metric Item')).toBeInTheDocument();
  });
});
