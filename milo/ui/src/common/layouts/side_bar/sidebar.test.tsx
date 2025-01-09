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

import { act, render, screen } from '@testing-library/react';

import { useEstablishProjectCtx } from '@/common/components/page_meta';
import {
  QueryTreesResponse,
  TreesClientImpl,
} from '@/proto/go.chromium.org/luci/tree_status/proto/v1/trees.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { Sidebar } from './sidebar';

interface ProjectSetterProps {
  readonly project: string;
}

// eslint-disable-next-line jest/no-export
export function ProjectSetter({ project }: ProjectSetterProps) {
  useEstablishProjectCtx(project);
  return <></>;
}

describe('Sidebar', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('given an empty project, should display builders search', async () => {
    render(
      <FakeContextProvider>
        <Sidebar open={true} />
      </FakeContextProvider>,
    );
    await screen.findByRole('complementary');
    expect(screen.getByText('Builder search')).toBeInTheDocument();
    expect(screen.queryByText('Builders')).toBeNull();
  });

  it('should display a list of navigation items for a project', async () => {
    render(
      <FakeContextProvider>
        <ProjectSetter project="chrome" />
        <Sidebar open={true} />
      </FakeContextProvider>,
    );
    await screen.findByRole('complementary');
    expect(screen.getByText('Builder search')).toBeInTheDocument();
    expect(screen.getByText('Builders')).toBeInTheDocument();
    expect(screen.getByText('Builder groups (Consoles)')).toBeInTheDocument();
    expect(screen.getByText('Test history')).toBeInTheDocument();
    expect(screen.getByText('Test Analysis')).toBeInTheDocument();
    expect(screen.getByText('Clusters')).toBeInTheDocument();
    expect(screen.getByText('Rules')).toBeInTheDocument();
    expect(screen.getByText('Sheriff-o-Matic')).toBeInTheDocument();
    expect(screen.getByText('ChromiumDash')).toBeInTheDocument();
  });

  it('should display tree status if one is available for a project', async () => {
    jest
      .spyOn(TreesClientImpl.prototype, 'QueryTrees')
      .mockImplementation(async (_req) => {
        return QueryTreesResponse.fromPartial({
          trees: [
            {
              name: 'trees/chromium',
            },
          ],
        });
      });
    render(
      <FakeContextProvider>
        <ProjectSetter project="chrome" />
        <Sidebar open={true} />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    await screen.findByRole('complementary');
    expect(screen.getByText('Builder search')).toBeInTheDocument();
    expect(screen.getByText('Builders')).toBeInTheDocument();
    expect(screen.getByText('Builder groups (Consoles)')).toBeInTheDocument();
    expect(screen.getByText('Test history')).toBeInTheDocument();
    expect(screen.getByText('Test Analysis')).toBeInTheDocument();
    expect(screen.getByText('Clusters')).toBeInTheDocument();
    expect(screen.getByText('Rules')).toBeInTheDocument();
    expect(screen.getByText('Sheriff-o-Matic')).toBeInTheDocument();
    expect(screen.getByText('Tree status')).toBeInTheDocument();
    expect(screen.getByText('ChromiumDash')).toBeInTheDocument();
  });

  it('should not display tree status if none is available for a project', async () => {
    jest
      .spyOn(TreesClientImpl.prototype, 'QueryTrees')
      .mockImplementation(async (_req) => {
        return QueryTreesResponse.fromPartial({
          trees: [],
        });
      });
    render(
      <FakeContextProvider>
        <ProjectSetter project="chrome" />
        <Sidebar open={true} />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    await screen.findByRole('complementary');
    expect(screen.getByText('Builder search')).toBeInTheDocument();
    expect(screen.getByText('Builders')).toBeInTheDocument();
    expect(screen.getByText('Builder groups (Consoles)')).toBeInTheDocument();
    expect(screen.getByText('Test history')).toBeInTheDocument();
    expect(screen.getByText('Test Analysis')).toBeInTheDocument();
    expect(screen.getByText('Clusters')).toBeInTheDocument();
    expect(screen.getByText('Rules')).toBeInTheDocument();
    expect(screen.getByText('Sheriff-o-Matic')).toBeInTheDocument();
    expect(screen.queryByText('Tree status')).toBeNull();
    expect(screen.getByText('ChromiumDash')).toBeInTheDocument();
  });

  it('should not display tree status if more than one is available for a project', async () => {
    jest
      .spyOn(TreesClientImpl.prototype, 'QueryTrees')
      .mockImplementation(async (_req) => {
        return QueryTreesResponse.fromPartial({
          trees: [
            {
              name: 'trees/chromium',
            },
            {
              name: 'trees/chromium1',
            },
          ],
        });
      });
    render(
      <FakeContextProvider>
        <ProjectSetter project="chrome" />
        <Sidebar open={true} />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    await screen.findByRole('complementary');
    expect(screen.getByText('Builder search')).toBeInTheDocument();
    expect(screen.getByText('Builders')).toBeInTheDocument();
    expect(screen.getByText('Builder groups (Consoles)')).toBeInTheDocument();
    expect(screen.getByText('Test history')).toBeInTheDocument();
    expect(screen.getByText('Test Analysis')).toBeInTheDocument();
    expect(screen.getByText('Clusters')).toBeInTheDocument();
    expect(screen.getByText('Rules')).toBeInTheDocument();
    expect(screen.getByText('Sheriff-o-Matic')).toBeInTheDocument();
    expect(screen.queryByText('Tree status')).toBeNull();
    expect(screen.getByText('ChromiumDash')).toBeInTheDocument();
  });
});
