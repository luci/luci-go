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

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { InvocationArtifactsExplorer } from './invocation_artifacts_explorer';

// Mock dependencies
jest.mock('@/generic_libs/hooks/synced_search_params', () => ({
  useSyncedSearchParams: jest.fn(),
}));
jest.mock('./invocation_artifacts_loader', () => ({
  InvocationArtifactsLoader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

jest.mock(
  '@/test_investigation/components/common/artifacts/tree/context/provider',
  () => ({
    ArtifactFilterProvider: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
  }),
);

// Mock resize panels to avoid structural issues in tests
jest.mock('react-resizable-panels', () => ({
  PanelGroup: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  Panel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  PanelResizeHandle: () => <div>ResizeHandle</div>,
}));

jest.mock(
  '@/test_investigation/components/common/artifacts/tree/artifact_tree_layout',
  () => ({
    ArtifactsTreeLayout: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
  }),
);

jest.mock(
  '@/test_investigation/components/common/artifacts/tree/artifact_tree_view/artifact_tree_view',
  () => ({
    ArtifactTreeView: () => <div>ArtifactTreeView</div>,
  }),
);

jest.mock(
  '@/test_investigation/components/common/artifacts/content/artifact_content_view',
  () => ({
    ArtifactContentView: () => <div>ArtifactContentView</div>,
  }),
);

jest.mock('@/common/components/sanitized_html', () => ({
  SanitizedHtml: ({ html }: { html: string }) => (
    <div data-testid="sanitized-html">{html}</div>
  ),
}));

jest.mock('@/common/tools/markdown/utils', () => ({
  renderMarkdown: (md: string) => `rendered-${md}`,
}));

// Mock context to control selectedNode
const mockUseArtifacts = jest.fn();
jest.mock(
  '@/test_investigation/components/common/artifacts/context/context',
  () => ({
    useArtifacts: () => mockUseArtifacts(),
  }),
);

const mockUseSyncedSearchParams = useSyncedSearchParams as jest.Mock;

describe('InvocationArtifactsExplorer', () => {
  beforeEach(() => {
    mockUseSyncedSearchParams.mockReturnValue([
      new URLSearchParams(),
      jest.fn(),
    ]);

    mockUseArtifacts.mockReturnValue({
      selectedNode: null,
      invocation: { name: 'invocations/inv-1' },
      nodes: [{ id: 'n1', name: 'node' }],
    });
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it('renders tree view', () => {
    render(<InvocationArtifactsExplorer />);
    expect(screen.getByText('ArtifactTreeView')).toBeInTheDocument();
  });

  it('renders prompt when no artifact is selected', () => {
    render(<InvocationArtifactsExplorer />);
    expect(screen.getByText('Select an artifact to view.')).toBeInTheDocument();
  });

  it('renders empty message when no nodes', () => {
    mockUseArtifacts.mockReturnValue({
      selectedNode: null,
      invocation: { name: 'invocations/inv-1' },
      nodes: [],
    });
    render(<InvocationArtifactsExplorer />);
    expect(screen.getByText('No artifacts to display.')).toBeInTheDocument();
    expect(screen.queryByText('ArtifactTreeView')).not.toBeInTheDocument();
  });

  it('renders no summary available when summary node is selected without markdown', () => {
    mockUseArtifacts.mockReturnValue({
      selectedNode: { isSummary: true, artifact: null },
      invocation: { name: 'invocations/inv-1' },
      nodes: [{ id: 'n1' }],
    });
    render(<InvocationArtifactsExplorer />);
    expect(screen.getByText('No summary available.')).toBeInTheDocument();
  });

  it('renders content view when artifact is selected', () => {
    mockUseArtifacts.mockReturnValue({
      selectedNode: { isSummary: false, artifact: { name: 'foo' } },
      invocation: { name: 'invocations/inv-1' },
      nodes: [{ id: 'n1' }],
    });
    render(<InvocationArtifactsExplorer />);
    expect(screen.getByText('ArtifactContentView')).toBeInTheDocument();
  });

  it('renders markdown summary when RootInvocation has summary', () => {
    mockUseArtifacts.mockReturnValue({
      selectedNode: { isSummary: true, artifact: null },
      invocation: {
        name: 'invocations/inv-root',
        rootInvocationId: 'inv-root',
        summaryMarkdown: 'some summary',
      },
      nodes: [{ id: 'n1' }],
    });
    render(<InvocationArtifactsExplorer />);
    expect(screen.getByTestId('sanitized-html')).toHaveTextContent(
      'rendered-some summary',
    );
  });
});
