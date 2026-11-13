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

import '@testing-library/jest-dom';
import { render, screen } from '@testing-library/react';

import { ExternalCaptionLink } from './external_caption_link';

describe('ExternalCaptionLink', () => {
  it('opens in a new tab without leaking the opener', () => {
    render(
      <ExternalCaptionLink href="https://example.com">
        Example
      </ExternalCaptionLink>,
    );
    const link = screen.getByRole('link', { name: 'Example' });
    expect(link).toHaveAttribute('href', 'https://example.com');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('merges custom styles with the defaults', () => {
    render(
      <ExternalCaptionLink
        href="https://example.com"
        sx={{ fontFamily: 'monospace' }}
      >
        Example
      </ExternalCaptionLink>,
    );
    expect(screen.getByRole('link', { name: 'Example' })).toHaveStyle({
      fontFamily: 'monospace',
      textDecoration: 'none',
    });
  });
});
