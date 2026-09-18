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
import userEvent from '@testing-library/user-event';

import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import { MarkdownSnippet } from './markdown_snippet';

describe('<MarkdownSnippet />', () => {
  it('renders nothing when markdown is empty', () => {
    const { container } = render(
      <FakeAuthStateProvider>
        <MarkdownSnippet markdown="" copyKind="test" />
      </FakeAuthStateProvider>,
    );
    expect(container.firstChild).toBeNull();
  });

  it('renders inline snippet with label when not collapsible', async () => {
    const user = userEvent.setup();
    render(
      <FakeAuthStateProvider>
        <MarkdownSnippet
          label="Changelog (Markdown for Buganizer):"
          markdown="**Updated field:** `a` ➔ `b`"
          copyKind="changelog"
        />
      </FakeAuthStateProvider>,
    );

    expect(
      screen.getByText('Changelog (Markdown for Buganizer):'),
    ).toBeVisible();
    expect(screen.getByText('**Updated field:** `a` ➔ `b`')).toBeVisible();

    const copyBtn = screen.getByRole('button', { name: 'Copy to clipboard' });
    await user.click(copyBtn);

    const copiedText = await navigator.clipboard.readText();
    expect(copiedText).toBe('**Updated field:** `a` ➔ `b`');
  });

  it('renders collapsible header collapsed by default and allows copying directly or expanding', async () => {
    const user = userEvent.setup();
    render(
      <FakeAuthStateProvider>
        <MarkdownSnippet
          label="Autorepair results (Markdown for Buganizer):"
          markdown={
            '**Autorepair results:**\n* [dut-1](http://localhost/dut-1)'
          }
          copyKind="autorepair_results_markdown"
          collapsible
        />
      </FakeAuthStateProvider>,
    );

    const expandBtn = screen.getByRole('button', {
      name: 'Autorepair results (Markdown for Buganizer):',
    });
    expect(expandBtn).toBeVisible();
    expect(expandBtn).toHaveAttribute('aria-expanded', 'false');
    expect(
      screen.queryByText(
        '**Autorepair results:**\n* [dut-1](http://localhost/dut-1)',
      ),
    ).not.toBeInTheDocument();

    // Click copy button while collapsed
    const copyBtn = screen.getByRole('button', { name: 'Copy markdown' });
    await user.click(copyBtn);
    expect(await navigator.clipboard.readText()).toBe(
      '**Autorepair results:**\n* [dut-1](http://localhost/dut-1)',
    );
    expect(screen.getByRole('button', { name: 'Copied' })).toBeVisible();
    expect(expandBtn).toHaveAttribute('aria-expanded', 'false');

    // Expand to view code snippet
    await user.click(expandBtn);
    expect(expandBtn).toHaveAttribute('aria-expanded', 'true');
    const collapseId = expandBtn.getAttribute('aria-controls');
    expect(collapseId).toBeTruthy();
    expect(document.getElementById(collapseId!)).toBeInTheDocument();
    expect(
      screen.getByText(/\* \[dut-1\]\(http:\/\/localhost\/dut-1\)/),
    ).toBeVisible();
  });

  it('logs a warning when clipboard write fails', async () => {
    const user = userEvent.setup();
    const warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => {});
    const writeTextSpy = jest
      .spyOn(navigator.clipboard, 'writeText')
      .mockRejectedValueOnce(new Error('Clipboard blocked'));

    render(
      <FakeAuthStateProvider>
        <MarkdownSnippet
          label="Autorepair results (Markdown for Buganizer):"
          markdown="**Autorepair results:**"
          copyKind="autorepair_results_markdown"
          collapsible
        />
      </FakeAuthStateProvider>,
    );

    const copyBtn = screen.getByRole('button', { name: 'Copy markdown' });
    await user.click(copyBtn);

    expect(warnSpy).toHaveBeenCalledWith(
      'Failed to copy markdown to clipboard:',
      expect.any(Error),
    );

    writeTextSpy.mockRestore();
    warnSpy.mockRestore();
  });
});
