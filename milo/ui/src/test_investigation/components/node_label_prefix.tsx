// Copyright 2026 The LUCI Authors.
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

import { Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { useSchemaClient } from '@/common/hooks/prpc_clients';
import { TestNavigationTreeNode } from '@/test_investigation/components/test_navigation_drawer/types';
import { StructuredTreeLevel } from '@/test_investigation/utils/drawer_tree_utils';

interface NodeLabelPrefixProps {
  node: TestNavigationTreeNode;
}

export function NodeLabelPrefix({ node }: NodeLabelPrefixProps) {
  const schemaClient = useSchemaClient();
  const { data } = useQuery({
    ...schemaClient.GetScheme.query({
      name: `schema/schemes/${node.moduleScheme}`,
    }),
    enabled: !!node.moduleScheme,
  });

  if (!node.isStructured) {
    return null;
  }

  const coarseLabel = data?.coarse?.humanReadableName || 'Package';
  const fineLabel = data?.fine?.humanReadableName || 'Class';
  const caseLabel = data?.case?.humanReadableName || 'Case';

  let prefix = '';
  if (node.testVariant) {
    prefix = 'Case: ';
  } else {
    switch (node.level) {
      case StructuredTreeLevel.Module:
        prefix = 'Module: ';
        break;
      case StructuredTreeLevel.Coarse:
        prefix = `${coarseLabel}: `;
        break;
      case StructuredTreeLevel.Fine:
        prefix = `${fineLabel}: `;
        break;
      case StructuredTreeLevel.Case:
        prefix = `${caseLabel}: `;
        break;
    }
  }

  if (!prefix) return null;

  return (
    <Typography
      component="span"
      variant="inherit"
      sx={{ mr: 0.5, color: 'text.secondary' }}
    >
      {prefix}
    </Typography>
  );
}
