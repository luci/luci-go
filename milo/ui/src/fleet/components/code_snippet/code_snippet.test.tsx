// Copyright 2025 The LUCI Authors.
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
import userEvent, { UserEvent } from '@testing-library/user-event';

import CodeSnippet from './code_snippet';

async function pause(delay: number) {
  return await new Promise((resolve) => setTimeout(resolve, delay));
}

describe('<CodeSnippet />', () => {
  let user: UserEvent;

  beforeEach(() => {
    user = userEvent.setup();
  });

  it('renders text to copy', async () => {
    render(<CodeSnippet copyText="Lorem ipsum" />);

    expect(screen.getByText('Lorem ipsum')).toBeVisible();
  });

  it('clicking on the copy icon makes text available in clipboard', async () => {
    render(<CodeSnippet copyText="Lorem ipsum" />);

    const copyBtn = screen.getByRole('button', { name: 'Copy to clipboard' });
    expect(copyBtn).toBeInTheDocument();
    await user.click(copyBtn);
    await pause(10);

    const copiedText = await navigator.clipboard.readText();
    expect(copiedText).toEqual('Lorem ipsum');
  });

  it('display one text, use other to be copied, check if copy text is available in clipboard', async () => {
    render(<CodeSnippet copyText="Lorem ipsum" displayText="Dolor sit amet" />);

    expect(screen.queryByText('Lorem ipsum')).not.toBeInTheDocument();
    expect(screen.queryByText('Dolor sit amet')).toBeVisible();

    await user.click(screen.getByRole('button', { name: 'Copy to clipboard' }));
    await pause(10);

    const copiedText = await navigator.clipboard.readText();
    expect(copiedText).toEqual('Lorem ipsum');
  });
});
