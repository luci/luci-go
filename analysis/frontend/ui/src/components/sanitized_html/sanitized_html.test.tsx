// Copyright 2023 The LUCI Authors.
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

import { SanitizedHtml } from './sanitized_html';

const DIRTY_HTML = `
<div data-testid="safe-content">Safe Content</div>
<a data-testid="unsafe-link" href="javascript:alert('unsafe')">unsafe_link</a>
<script data-testid="unsafe-script">
  throw new Error('Unsafe content');
</script>
`;

describe('SanitizedHtml', () => {
  it('should sanitize HTML', () => {
    render(<SanitizedHtml html={DIRTY_HTML} />);

    const safeContent = screen.getByTestId('safe-content');
    expect(safeContent).toBeInTheDocument();
    expect(safeContent).toHaveTextContent('Safe Content');

    const unsafeLink = screen.getByTestId('unsafe-link');
    expect(unsafeLink).toBeInTheDocument();
    expect(unsafeLink).not.toHaveAttribute('href');
    expect(unsafeLink).toHaveTextContent('unsafe_link');

    const unsafeScript = screen.queryByTestId('unsafe-script');
    expect(unsafeScript).not.toBeInTheDocument();
  });
});
