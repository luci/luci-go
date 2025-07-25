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

import { VersionInfo } from './version_info';

describe('VersionInfo', () => {
  it('should render the version', () => {
    render(<VersionInfo version="12345-abcdef" />);
    expect(screen.getByText(/Version:/)).toBeInTheDocument();
    expect(screen.getByText('12345-abcdef')).toBeInTheDocument();
  });

  it('should have a link to the commit log', () => {
    render(<VersionInfo version="12345-abcdef" />);
    const link = screen.getByRole('link');
    expect(link).toHaveAttribute(
      'href',
      'https://chromium.googlesource.com/infra/luci/luci-go/+log/abcdef',
    );
  });
});
