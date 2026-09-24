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

import {
  fireEvent,
  render,
  screen,
  waitForElementToBeRemoved,
} from '@testing-library/react';

import { EllipsisTooltip } from './index';

describe('<EllipsisTooltip />', () => {
  it('does not open tooltip when content does not overflow', () => {
    render(<EllipsisTooltip tooltip="Full text">Short text</EllipsisTooltip>);
    const box = screen.getByText('Short text');

    Object.defineProperty(box, 'scrollWidth', {
      configurable: true,
      value: 100,
    });
    Object.defineProperty(box, 'clientWidth', {
      configurable: true,
      value: 100,
    });

    fireEvent.mouseEnter(box);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('opens tooltip lazily on hover when content overflows and closes on mouseLeave', async () => {
    render(
      <EllipsisTooltip tooltip="Overflowing full text">
        Truncated text
      </EllipsisTooltip>,
    );
    const box = screen.getByText('Truncated text');

    Object.defineProperty(box, 'scrollWidth', {
      configurable: true,
      value: 200,
    });
    Object.defineProperty(box, 'clientWidth', {
      configurable: true,
      value: 100,
    });

    fireEvent.mouseEnter(box);
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      'Overflowing full text',
    );

    fireEvent.mouseLeave(box);
    await waitForElementToBeRemoved(() => screen.queryByRole('tooltip'));
  });

  it('opens tooltip lazily on focus when content overflows and closes on blur', async () => {
    render(
      <EllipsisTooltip tooltip="Focus full text">Focus text</EllipsisTooltip>,
    );
    const box = screen.getByText('Focus text');

    Object.defineProperty(box, 'scrollWidth', {
      configurable: true,
      value: 200,
    });
    Object.defineProperty(box, 'clientWidth', {
      configurable: true,
      value: 100,
    });

    fireEvent.focus(box);
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      'Focus full text',
    );

    fireEvent.blur(box);
    await waitForElementToBeRemoved(() => screen.queryByRole('tooltip'));
  });

  it('opens tooltip lazily on hover when a nested child element overflows', async () => {
    render(
      <EllipsisTooltip tooltip="Middle truncated text">
        <span>Nested child text</span>
      </EllipsisTooltip>,
    );
    const box = screen.getByText('Nested child text').parentElement!;
    const child = screen.getByText('Nested child text');

    Object.defineProperty(box, 'scrollWidth', {
      configurable: true,
      value: 100,
    });
    Object.defineProperty(box, 'clientWidth', {
      configurable: true,
      value: 100,
    });
    Object.defineProperty(child, 'scrollWidth', {
      configurable: true,
      value: 250,
    });
    Object.defineProperty(child, 'clientWidth', {
      configurable: true,
      value: 100,
    });

    fireEvent.mouseEnter(box);
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      'Middle truncated text',
    );
  });
});
