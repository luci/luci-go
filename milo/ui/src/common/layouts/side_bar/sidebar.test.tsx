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

import { render, screen } from '@testing-library/react';

import { UiPage } from '@/common/constants/view';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { Sidebar } from './sidebar';

describe('Sidebar', () => {
  it('given an empty project, should display builders search', async () => {
    render(
      <FakeContextProvider
        pageMeta={{
          selectedPage: UiPage.BuilderSearch,
        }}
      >
        <Sidebar open={true} />
      </FakeContextProvider>,
    );
    await screen.findByRole('complementary');
    expect(screen.getByText('Builder search')).toBeInTheDocument();
    expect(screen.queryByText('Builders')).toBeNull();
  });

  it('should display a list of navigation items for a project', async () => {
    render(
      <FakeContextProvider
        pageMeta={{
          selectedPage: UiPage.Builders,
          project: 'chrome',
        }}
      >
        <Sidebar open={true} />
      </FakeContextProvider>,
    );
    await screen.findByRole('complementary');
    expect(screen.getByText('Builder search')).toBeInTheDocument();
    expect(screen.getByText('Builders')).toBeInTheDocument();
    expect(screen.getByText('Builder groups (Consoles)')).toBeInTheDocument();
    expect(screen.getByText('Test history')).toBeInTheDocument();
    expect(screen.getByText('Failure clusters')).toBeInTheDocument();
    expect(screen.getByText('Sheriff-o-Matic')).toBeInTheDocument();
    expect(screen.getByText('ChromiumDash')).toBeInTheDocument();
  });
});
