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

import { render, screen, waitFor } from '@testing-library/react';

import {
  TreeData,
  VirtualTreeNodeActions,
} from '@/common/components/log_viewer';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ArtifactTreeNodeData } from '../types';

import { ArtifactTreeNode } from './artifact_tree_node';

describe('<ArtifactTreeNode />', () => {
  it('given a folder row then should display folder icon', async () => {
    const fakeTreeData: TreeData<ArtifactTreeNodeData> = {
      id: 'folder1',
      isLeafNode: false,
      level: 0,
      name: 'Test Folder',
      isOpen: false,
      data: { id: 'folder1', name: 'Test Folder', children: [] },
      children: [],
      parent: undefined,
    };
    const fakeTreeContext: VirtualTreeNodeActions<ArtifactTreeNodeData> = {
      onNodeToggle: jest.fn(),
      onNodeSelect: jest.fn(),
      isSelected: false,
    };
    render(
      <FakeContextProvider>
        <ArtifactTreeNode
          index={0}
          row={fakeTreeData}
          context={fakeTreeContext}
        />
      </FakeContextProvider>,
    );

    waitFor(() => expect(screen.getByText('Test Folder')).toBeInTheDocument());
  });

  it('given a file row then should display file icon', async () => {
    const fakeTreeData: TreeData<ArtifactTreeNodeData> = {
      id: 'file1',
      isLeafNode: true,
      level: 0,
      name: 'Test File.txt',
      isOpen: false,
      data: {
        id: 'file1',
        name: 'Test File.txt',
        children: [],
        artifact: Artifact.fromPartial({
          name: 'Test File.txt',
          artifactId: 'file1.txt',
          contentType: 'text/plain',
        }),
      },
      children: [],
      parent: undefined,
    };
    const fakeTreeContext: VirtualTreeNodeActions<ArtifactTreeNodeData> = {
      onNodeToggle: jest.fn(),
      onNodeSelect: jest.fn(),
      isSelected: false,
    };

    render(
      <FakeContextProvider>
        <ArtifactTreeNode
          index={0}
          row={fakeTreeData}
          context={fakeTreeContext}
        />
      </FakeContextProvider>,
    );

    waitFor(() => expect(screen.getByText('file1.txt')).toBeInTheDocument());
  });
});
