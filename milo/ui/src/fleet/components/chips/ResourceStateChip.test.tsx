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

import { State } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/state.pb';

import { ResourceStateChip } from './ResourceStateChip';

describe('<ResourceStateChip />', () => {
  it('renders numeric state correctly with formatting and correct text', () => {
    render(<ResourceStateChip state={State.STATE_SERVING} />);
    expect(screen.getByText('SERVING')).toBeInTheDocument();
  });

  it('renders string state correctly with formatting and replacing underscores', () => {
    render(<ResourceStateChip state="STATE_NEEDS_REPAIR" />);
    expect(screen.getByText('NEEDS REPAIR')).toBeInTheDocument();
  });

  it('renders N/A for null or undefined state', () => {
    render(<ResourceStateChip state={undefined} />);
    expect(screen.getByText('N/A')).toBeInTheDocument();
  });
});
