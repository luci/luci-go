// Copyright 2022 The LUCI Authors.
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

import '@testing-library/jest-dom';

import { fireEvent, render, screen } from '@testing-library/react';

import HelpTooltip from './help_tooltip';

describe('Test HelpTooltip component', () => {
  it('given a title, should display it', async () => {
    render(<HelpTooltip text="I can help you" />);

    await screen.findByRole('button');
    const button = screen.getByRole('button');
    fireEvent.mouseOver(button);
    await screen.findByText('I can help you');
    expect(screen.getByText('I can help you')).toBeInTheDocument();
  });
});
