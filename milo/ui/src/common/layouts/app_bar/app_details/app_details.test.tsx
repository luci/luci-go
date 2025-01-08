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

import { usePageId, useProject } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AppDetails } from './app_details';

interface ProjectSetterProps {
  readonly project: string;
}

function ProjectSetter({ project }: ProjectSetterProps) {
  useProject(project);
  return <></>;
}

interface PageSetterProps {
  readonly pageId: UiPage;
}

function PageSetter({ pageId }: PageSetterProps) {
  usePageId(pageId);
  return <></>;
}

describe('AppDetails', () => {
  it('should display app name given no data', async () => {
    render(
      <FakeContextProvider>
        <AppDetails open={true} handleSidebarChanged={(_: boolean) => {}} />
      </FakeContextProvider>,
    );
    await screen.findByLabelText('menu');
    expect(screen.getByText('LUCI')).toBeInTheDocument();
  });

  it('should display selected page', async () => {
    render(
      <FakeContextProvider>
        <PageSetter pageId={UiPage.Builders} />
        <AppDetails open={true} handleSidebarChanged={(_: boolean) => {}} />
      </FakeContextProvider>,
    );
    await screen.findByLabelText('menu');
    expect(screen.getByText('LUCI')).toBeInTheDocument();
    expect(screen.getByText('Builders')).toBeInTheDocument();
  });

  it('should display project', async () => {
    render(
      <FakeContextProvider>
        <ProjectSetter project="chrome" />
        <AppDetails open={true} handleSidebarChanged={(_: boolean) => {}} />
      </FakeContextProvider>,
    );
    await screen.findByLabelText('menu');
    expect(screen.getByText('LUCI')).toBeInTheDocument();
    expect(screen.getByText('chrome')).toBeInTheDocument();
  });
});
