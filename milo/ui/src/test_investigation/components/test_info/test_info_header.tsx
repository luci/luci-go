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

import { Box, Button, Divider, Typography } from '@mui/material';

import { PageSummaryLine } from '@/common/components/page_summary_line';
import { PageTitle } from '@/common/components/page_title';
import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';
import {
  useInvocation,
  useProject,
  useTestVariant,
} from '@/test_investigation/context';
import { getFullMethodName } from '@/test_investigation/utils/test_info_utils';

import { VariantDisplay } from '../common/variant_display';

import { useDrawerWrapper, useTestVariantBranch } from './context';
import { TestInfoBreadcrumbs } from './test_info_breadcrumbs';
import { TestInfoMarkers } from './test_info_markers';

export function TestInfoHeader() {
  const testVariant = useTestVariant();
  const invocation = useInvocation();
  const project = useProject();
  const testVariantBranch = useTestVariantBranch();
  const { onToggleDrawer } = useDrawerWrapper();

  const testDisplayName =
    testVariant?.testIdStructured?.caseName ||
    testVariant.testMetadata?.name ||
    testVariant.testId;

  return (
    <Box
      sx={{ display: 'flex', flexDirection: 'column', gap: 1, px: 3, py: 2 }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 2,
        }}
      >
        <Button
          id="test-investigation-toggle-drawer"
          variant="contained"
          onClick={onToggleDrawer}
          sx={{
            fontWeight: 'bold',
            height: '32px',
          }}
        >
          View Tests
        </Button>
        <Divider orientation="vertical" flexItem />
        <TestInfoBreadcrumbs
          invocation={invocation.name}
          testIdStructured={testVariant?.testIdStructured || undefined}
        />
      </Box>
      <PageTitle viewName="Test case" resourceName={testDisplayName} />
      {testVariant?.testIdStructured?.moduleScheme === 'junit' && (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, pt: 0 }}>
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            {getFullMethodName(testVariant)}
          </Typography>
          <CopyToClipboard
            textToCopy={getFullMethodName(testVariant)}
          ></CopyToClipboard>
        </Box>
      )}
      <PageSummaryLine>
        <TestInfoMarkers
          invocation={invocation}
          project={project}
          testVariant={testVariant}
          testVariantBranch={testVariantBranch}
        />
        <VariantDisplay variantDef={testVariant.variant?.def} />
      </PageSummaryLine>
    </Box>
  );
}
