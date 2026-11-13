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

import { SafeViewLink } from './safe_view_link';

describe('SafeViewLink', () => {
  it.each([
    'https://android-build.corp.google.com/builds/submitted/123/target/latest',
    'https://ci.chromium.org/ui/p/chromium/builders/try/linux-rel/123',
    'https://cr-buildbucket.appspot.com/build/8665224485559193713',
    'http://ab/12345',
    'http://localhost:8080/x',
  ])('renders %s as a link', (url) => {
    render(<SafeViewLink url={url} />);

    const link = screen.getByRole('link', { name: url });
    expect(link).toHaveAttribute('href', url);
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener');
  });

  it.each([
    'javascript:alert(1)',
    'JavaScript:alert(1)',
    'java\tscript:alert(1)',
    'data:text/html,<script>alert(1)</script>',
    '/ui/relative/path',
    'not a url',
  ])('renders %j as plain text', (url) => {
    const { container } = render(<SafeViewLink url={url} />);

    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    expect(container).toHaveTextContent(url.replace(/\s+/g, ' '));
  });
});
