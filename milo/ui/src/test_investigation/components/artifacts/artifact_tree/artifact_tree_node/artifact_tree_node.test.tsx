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

import { yellow } from '@mui/material/colors';
import { render, screen, waitFor } from '@testing-library/react';

import {
  TreeData,
  VirtualTreeNodeActions,
} from '@/common/components/log_viewer';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { InvocationProvider } from '@/test_investigation/context';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ArtifactTreeNodeData } from '../../types';

import { ArtifactTreeNode } from './artifact_tree_node';

const MOCK_RAW_INVOCATION_ID = 'inv-id-123';
const MOCK_PROJECT_ID = 'test-project';

describe('<ArtifactTreeNode />', () => {
  let mockInvocation: Invocation;
  let mockTreeContext: VirtualTreeNodeActions<ArtifactTreeNodeData>;

  beforeEach(() => {
    mockInvocation = Invocation.fromPartial({
      realm: `${MOCK_PROJECT_ID}:some-realm`,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
      name: 'invocations/ants-build-12345',
    });
    mockTreeContext = {
      onNodeToggle: jest.fn(),
      onNodeSelect: jest.fn(),
      isSelected: false,
    };
  });

  const renderComponent = (
    fakeTreeData: TreeData<ArtifactTreeNodeData>,
    fakeInvocation?: Invocation,
  ) => {
    const inv = fakeInvocation || mockInvocation;
    return render(
      <FakeContextProvider>
        <InvocationProvider
          project="test-project"
          invocation={inv}
          rawInvocationId={MOCK_RAW_INVOCATION_ID}
          isLegacyInvocation
        >
          <ArtifactTreeNode
            index={0}
            row={fakeTreeData}
            context={mockTreeContext}
          />
        </InvocationProvider>
      </FakeContextProvider>,
    );
  };

  it('given a folder row then should display folder icon and name', async () => {
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
    renderComponent(fakeTreeData);

    await waitFor(() =>
      expect(screen.getByText('Test Folder')).toBeInTheDocument(),
    );
    await waitFor(() => {
      expect(screen.getByTitle('folder-icon')).toBeInTheDocument();
    });
  });

  it('should display android icon for ants invocations', async () => {
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

    renderComponent(fakeTreeData);
    await waitFor(() => {
      expect(screen.getByTitle('adb-icon')).toBeInTheDocument();
    });
  });

  it('should not display android icon for non-ants invocations', async () => {
    const fakeInvocation: Invocation = Invocation.fromPartial({
      realm: `${MOCK_PROJECT_ID}:some-realm`,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
      name: 'invocations/build-12345',
    });

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

    renderComponent(fakeTreeData, fakeInvocation);
    expect(screen.queryByTitle('adb-icon')).toBeNull();
  });

  it('given a file row then should display file icon and artifactId', async () => {
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

    renderComponent(fakeTreeData);

    await waitFor(() => {
      expect(screen.getByText('Test File.txt')).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.getByTitle('file-icon')).toBeInTheDocument();
    });
  });

  it('given an image artifact then should display image icon', async () => {
    const fakeTreeData: TreeData<ArtifactTreeNodeData> = {
      id: 'file2',
      isLeafNode: true,
      level: 0,
      name: 'Test File.png',
      isOpen: false,
      data: {
        id: 'file1',
        name: 'Test File.png',
        children: [],
        artifact: Artifact.fromPartial({
          name: 'Test File.png',
          artifactId: 'image_diff',
          contentType: 'image/png',
        }),
      },
      children: [],
      parent: undefined,
    };

    renderComponent(fakeTreeData);

    await waitFor(() => {
      expect(screen.getByText('Test File.png')).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.getByTitle('image-icon')).toBeInTheDocument();
    });
  });

  it('should highlight matching text when highlightText prop is provided', () => {
    const nodeName = 'my-special-log-file.txt';
    const highlightTerm = 'log';

    const fakeTreeData: TreeData<ArtifactTreeNodeData> = {
      id: 'file-with-highlight',
      isLeafNode: true,
      level: 0,
      name: nodeName,
      isOpen: false,
      data: {
        id: 'file-with-highlight',
        name: nodeName,
        children: [],
        artifact: Artifact.fromPartial({
          name: `invocations/inv1/tests/test1/results/result1/artifacts/${nodeName}`,
          artifactId: nodeName,
          contentType: 'text/plain',
        }),
      },
      children: [],
      parent: undefined,
    };
    const fakeTreeContext: VirtualTreeNodeActions<ArtifactTreeNodeData> = {
      onNodeSelect: jest.fn(),
      isSelected: false,
    };

    render(
      <FakeContextProvider>
        <InvocationProvider
          project="test-project"
          invocation={mockInvocation}
          rawInvocationId={MOCK_RAW_INVOCATION_ID}
          isLegacyInvocation
        >
          <ArtifactTreeNode
            index={0}
            row={fakeTreeData}
            context={fakeTreeContext}
            highlightText={highlightTerm}
          />
        </InvocationProvider>
      </FakeContextProvider>,
    );

    const highlightedTextElement = screen.getByText(highlightTerm);
    expect(highlightedTextElement).toBeInTheDocument();

    expect(highlightedTextElement.tagName).toBe('STRONG');

    const parentWrapper = highlightedTextElement.parentElement;
    expect(parentWrapper).toHaveStyle(`background-color: ${yellow[200]}`);
  });

  it('given an empty folder row (leaf but no artifact) then should display folder icon', async () => {
    const fakeTreeData: TreeData<ArtifactTreeNodeData> = {
      id: 'emptyFolder',
      isLeafNode: true,
      level: 0,
      name: 'Empty Folder',
      isOpen: false,
      data: { id: 'emptyFolder', name: 'Empty Folder', children: [] },
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

    waitFor(() => expect(screen.getByText('Empty Folder')).toBeInTheDocument());
  });
});
