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

import { Box, Typography, Link } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { PageSummaryLine } from '@/common/components/page_summary_line';
import { VerdictStatusIcon } from '@/common/components/verdict_status_icon';
import { useSchemaClient } from '@/common/hooks/prpc_clients';
import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';
import {
  useInvocation,
  useTestVariant,
  useProject,
} from '@/test_investigation/context';
import { getDisplayInvocationId } from '@/test_investigation/utils/invocation_utils';
import { getFullMethodName } from '@/test_investigation/utils/test_info_utils';

import { VariantDisplay } from '../common/variant_display';

import { useTestVariantBranch } from './context';
import { TestInfoMarkers } from './test_info_markers';

function TestNameTooltip() {
  const invocation = useInvocation();
  const testVariant = useTestVariant();
  const structured = testVariant.testIdStructured;
  const invID = getDisplayInvocationId(invocation);

  const schemaClient = useSchemaClient();
  const schemeId = structured?.moduleScheme;
  const { data: schema } = useQuery({
    ...schemaClient.GetScheme.query({ name: `schema/schemes/${schemeId}` }),
    enabled: !!schemeId,
  });

  const coarseLabel = schema?.coarse?.humanReadableName || 'Coarse Name';
  const fineLabel = schema?.fine?.humanReadableName || 'Fine Name';

  const testDisplayName =
    testVariant?.testIdStructured?.caseName ||
    testVariant.testMetadata?.name ||
    testVariant.testId;

  const fullMethodName = getFullMethodName(testVariant);

  return (
    <Box
      sx={{
        p: 1.5,
        minWidth: '400px',
        display: 'grid',
        gridTemplateColumns: 'auto 1fr',
        columnGap: 2,
        rowGap: 0.75,
        alignItems: 'center',
      }}
    >
      <Typography variant="caption" sx={{ color: 'text.secondary' }}>
        Invocation:
      </Typography>
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <Link
          target="_blank"
          href={`/ui/test-investigate/invocations/${invID}`}
          sx={{ overflow: 'hidden', textOverflow: 'ellipsis' }}
        >
          {invID}
        </Link>
        <CopyToClipboard
          textToCopy={invID}
          aria-label="Copy invocation ID."
          sx={{ ml: 0.5 }}
        />
      </Box>

      {structured?.moduleName && (
        <>
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            Module:
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <Typography variant="caption">{structured.moduleName}</Typography>
            <CopyToClipboard
              textToCopy={structured.moduleName}
              aria-label="Copy module name."
              sx={{ ml: 0.5 }}
            />
          </Box>
        </>
      )}

      {structured?.coarseName && (
        <>
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            {coarseLabel}:
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <Typography variant="caption">{structured.coarseName}</Typography>
            <CopyToClipboard
              textToCopy={structured.coarseName}
              aria-label={`Copy ${coarseLabel}.`}
              sx={{ ml: 0.5 }}
            />
          </Box>
        </>
      )}

      {structured?.fineName && (
        <>
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            {fineLabel}:
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <Typography variant="caption">{structured.fineName}</Typography>
            <CopyToClipboard
              textToCopy={structured.fineName}
              aria-label={`Copy ${fineLabel}.`}
              sx={{ ml: 0.5 }}
            />
          </Box>
        </>
      )}

      <Typography variant="caption" sx={{ color: 'text.secondary' }}>
        Test case:
      </Typography>
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <Typography variant="caption">{testDisplayName}</Typography>
        <CopyToClipboard
          textToCopy={testDisplayName}
          aria-label="Copy test case."
          sx={{ ml: 0.5 }}
        />
      </Box>

      {fullMethodName && (
        <Box
          sx={{
            gridColumn: '1 / span 2',
            display: 'flex',
            alignItems: 'center',
            gap: 0.5,
            mt: 0.5,
            pt: 0.5,
            borderTop: '1px solid',
            borderColor: 'divider',
          }}
        >
          <Typography
            variant="caption"
            sx={{ color: 'text.secondary', wordBreak: 'break-all' }}
          >
            {fullMethodName}
          </Typography>
          <CopyToClipboard textToCopy={fullMethodName} />
        </Box>
      )}
    </Box>
  );
}

export function CollapsedTestInfoHeader() {
  const testVariant = useTestVariant();
  const invocation = useInvocation();
  const project = useProject();
  const testVariantBranch = useTestVariantBranch();

  const testDisplayName =
    testVariant?.testIdStructured?.caseName ||
    testVariant.testMetadata?.name ||
    testVariant.testId;

  return (
    <Box
      sx={{
        borderBottom: '1px solid',
        borderColor: 'divider',
        backgroundColor: 'background.paper',
        boxShadow: '0 2px 4px rgba(0,0,0,0.1)',
      }}
    >
      <PageSummaryLine
        noWrap
        sx={{
          height: '48px',
          pl: 3,
          pr: 2,
          overflow: 'hidden',
          // gap: 1.5,
        }}
      >
        <HtmlTooltip title={<TestNameTooltip />}>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1,
              cursor: 'help',
              flexShrink: 0,
              minWidth: 0,
            }}
          >
            <VerdictStatusIcon
              statusV2={testVariant.statusV2}
              statusOverride={testVariant.statusOverride}
            />
            <Typography
              variant="subtitle1"
              sx={{
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                maxWidth: '500px',
                fontWeight: 500,
              }}
            >
              Test case: {testDisplayName}
            </Typography>
          </Box>
        </HtmlTooltip>

        <TestInfoMarkers
          invocation={invocation}
          project={project}
          testVariant={testVariant}
          testVariantBranch={testVariantBranch}
        />
        <VariantDisplay variantDef={testVariant.variant?.def} responsive />
      </PageSummaryLine>
    </Box>
  );
}
