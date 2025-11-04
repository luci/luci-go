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
import { MemoryRouter, Route, Routes } from 'react-router';

import { PlatformSelector } from './platform_selector';

describe('PlatformSelector', () => {
  it('renders the platform selector button pages that support platforms', () => {
    render(
      <MemoryRouter initialEntries={['/repairs/android']}>
        <Routes>
          <Route path="/repairs/:platform" element={<PlatformSelector />} />
        </Routes>
      </MemoryRouter>,
    );
    expect(screen.getByRole('button')).toBeInTheDocument();
  });

  it('renders the platform selector button even with an invalid platform', () => {
    render(
      <MemoryRouter initialEntries={['/repairs/invalidplatform']}>
        <Routes>
          <Route path="/repairs/:platform" element={<PlatformSelector />} />
        </Routes>
      </MemoryRouter>,
    );
    expect(screen.getByRole('button')).toBeInTheDocument();
  });

  it('does not render the platform selector button on other pages', () => {
    render(
      <MemoryRouter initialEntries={['/randompage']}>
        <Routes>
          <Route path="/test/:platform" element={<PlatformSelector />} />
        </Routes>
      </MemoryRouter>,
    );
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
