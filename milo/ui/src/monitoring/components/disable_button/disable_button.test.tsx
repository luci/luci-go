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

import { DisableTestButton } from './disable_button';

describe('disable_button', () => {
  it('renders', () => {
    render(<DisableTestButton failureBBID={'1234'} testID={'test.Example'} />);
    expect(screen.getByText('Disable')).toBeInTheDocument();
  });

  // navigator.clipboard is not supported, so can't check any more than this.
});
