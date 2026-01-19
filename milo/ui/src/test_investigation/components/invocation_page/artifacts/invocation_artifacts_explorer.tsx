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

import { Box, Typography } from '@mui/material';
import { useMemo, useState } from 'react';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ArtifactsSplitView } from '@/test_investigation/components/common/artifacts/artifacts_split_view';
import { useArtifacts } from '@/test_investigation/components/common/artifacts/context/context';
import { ArtifactTreeView } from '@/test_investigation/components/common/artifacts/tree/artifact_tree_view/artifact_tree_view';
import { ArtifactFilterProvider } from '@/test_investigation/components/common/artifacts/tree/context/provider';
import { InvocationArtifactSummary } from '@/test_investigation/components/invocation_page/artifacts/invocation_artifact_summary';
import { InvocationWorkUnitTreeView } from '@/test_investigation/components/invocation_page/artifacts/work_unit_tree';

import { InvocationArtifactsLoader } from './invocation_artifacts_loader';
import { LazyArtifactContentView } from './lazy_artifact_content_view';

export function InvocationArtifactsExplorer() {
  return (
    <ArtifactFilterProvider>
      <InvocationArtifactsLoader>
        <ExplorerContent />
      </InvocationArtifactsLoader>
    </ArtifactFilterProvider>
  );
}

function ExplorerContent() {
  const { selectedNode, nodes, invocation } = useArtifacts();
  // Lift state up for Work Unit Tree selection
  const [workUnitSelectedNode, setWorkUnitSelectedNode] =
    useState<typeof selectedNode>(null);

  const [searchParams] = useSyncedSearchParams();
  const viewMode =
    (searchParams.get('view') as 'artifacts' | 'work-units') || 'artifacts';

  const rootInvocationId = useMemo(() => {
    if (invocation?.name) {
      return invocation.name.split('/').pop() || '';
    }
    return '';
  }, [invocation]);

  const activeSelectedNode = selectedNode || workUnitSelectedNode;

  return (
    <ArtifactsSplitView
      sidebarContent={
        viewMode === 'artifacts' ? (
          nodes.length === 0 ? (
            <Box
              sx={{
                p: 2,
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                height: '100%',
              }}
            >
              <Typography color="text.secondary">
                No artifacts to display.
              </Typography>
            </Box>
          ) : (
            <ArtifactTreeView />
          )
        ) : (
          <InvocationWorkUnitTreeView
            rootInvocationId={rootInvocationId}
            onNodeSelect={setWorkUnitSelectedNode}
          />
        )
      }
      selectedNode={activeSelectedNode}
      renderContent={(node) => {
        if (node.isSummary) {
          return <InvocationArtifactSummary />;
        }
        if (node.artifact) {
          return <LazyArtifactContentView artifact={node.artifact} />;
        }
        return null; // Fallback to default empty state
      }}
    />
  );
}
