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

import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Link as MuiLink,
  ListItemButton,
  Box,
  Typography,
  Chip,
  IconButton,
  Skeleton,
  Tooltip,
} from '@mui/material';
import { Link } from 'react-router';

import {
  getStatusStyle,
  SemanticStatusType,
  StatusStyle,
} from '@/common/styles/status_styles';
import { generateTestInvestigateUrl } from '@/common/tools/url_utils';
import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { AggregationLevel } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { useRawInvocationId } from '@/test_investigation/context';
import {
  getSemanticStatusFromModuleStatus,
  getSemanticStatusFromVerdict,
} from '@/test_investigation/utils/drawer_tree_utils';

import { getVariantDefinitionString } from '../context/utils';

import {
  AggregationNode,
  IntermediateAggregationNode,
  LeafAggregationNode,
  useAggregationViewContext,
} from './context/context';

interface AggregationTreeItemProps {
  node: AggregationNode;
  style?: React.CSSProperties;
  measureRef?: (element: Element | null) => void;
}

export function AggregationTreeItem({
  node,
  style,
  measureRef,
}: AggregationTreeItemProps) {
  const { expandedIds, toggleExpansion, highlightedNodeId } =
    useAggregationViewContext();
  const rawInvocationId = useRawInvocationId();

  const isExpanded = expandedIds.has(node.id);
  const hasChildren = !node.isLeaf;
  const isSelected = highlightedNodeId === node.id;

  const handleToggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (hasChildren) {
      toggleExpansion(node.id);
    }
  };

  return (
    <div
      ref={measureRef as React.Ref<HTMLDivElement>}
      style={style}
      data-index={node.id}
    >
      <ListItemButton
        onClick={handleToggle}
        selected={isSelected}
        disableGutters
        sx={{
          pl: node.depth * 3 + 1,
          width: '100%',
          minHeight: '40px',
          py: 1,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'flex-start',
          borderBottom: 1,
          borderColor: 'divider',
          position: 'relative',
          backgroundColor: isSelected ? 'action.selected' : 'inherit',
          '&.Mui-selected': {
            backgroundColor: 'action.selected',
            border: '2px solid',
            borderColor: 'primary.light',
            '&:hover': {
              backgroundColor: 'action.hover',
            },
          },
        }}
      >
        <AggregationTreeItemContent
          node={node}
          rawInvocationId={rawInvocationId}
          isExpanded={isExpanded}
          handleToggle={handleToggle}
        />
      </ListItemButton>
    </div>
  );
}

interface StatusChipProps {
  label: React.ReactNode;
  statusStyle: StatusStyle;
  semanticStatus: SemanticStatusType;
}

function StatusChip({ label, statusStyle, semanticStatus }: StatusChipProps) {
  const isNeutral =
    semanticStatus === 'unknown' || semanticStatus === 'neutral';

  return (
    <Chip
      label={label}
      size="small"
      sx={{
        height: '24px',
        backgroundColor: statusStyle.backgroundColor,
        color: statusStyle.onBackgroundColor || statusStyle.textColor,
        border: isNeutral ? '1px solid' : '1px solid transparent',
        borderColor: isNeutral
          ? statusStyle.borderColor || 'divider'
          : 'transparent',
        fontWeight: 500,
        borderRadius: '12px',
        fontSize: '0.75rem',
      }}
    />
  );
}

// Status Logic:
// - Leaf: Use verdict status.
// - Module: Use module_status if available.
// - Others: No explicit status color generally (or neutral).
function getSemanticStatus(node: AggregationNode): SemanticStatusType {
  if (node.isLeaf) {
    if (node.verdict) {
      return getSemanticStatusFromVerdict(
        node.verdict.status as unknown as TestVerdict_Status,
      );
    }
    return 'unknown';
  }

  // For Module, use moduleStatus
  const moduleStatus = node.aggregationData?.moduleStatus;
  if (moduleStatus) {
    return getSemanticStatusFromModuleStatus(moduleStatus);
  }

  return 'unknown';
}

interface IntermediateTreeItemContentProps {
  node: IntermediateAggregationNode;
  isExpanded: boolean;
  handleToggle: (e: React.MouseEvent) => void;
}

function IntermediateTreeItemContent({
  node,
  isExpanded,
  handleToggle,
}: IntermediateTreeItemContentProps) {
  const label = node.label || 'Unknown';
  const counts = node.aggregationData?.verdictCounts;

  const total =
    (counts?.passed || 0) +
    (counts?.failed || 0) +
    (counts?.flaky || 0) +
    (counts?.skipped || 0) +
    (counts?.executionErrored || 0);

  const semanticStatus = getSemanticStatus(node);
  const statusStyle = getStatusStyle(semanticStatus);

  const parts = [];
  if (counts?.failed) parts.push(`${counts.failed} Failed`);
  if (counts?.executionErrored) parts.push(`${counts.executionErrored} Errors`);
  if (counts?.flaky) parts.push(`${counts.flaky} Flaky`);

  const summary = (() => {
    if (parts.length === 0) {
      if (counts?.passed) return `${counts.passed} Passed`;
      return 'No results';
    }
    const summaryText = parts.join(', ');
    return (
      <span>
        {summaryText}{' '}
        <Box
          component="span"
          sx={{ fontStyle: 'italic', color: 'text.secondary' }}
        >
          ({total} tests ran)
        </Box>
      </span>
    );
  })();

  // Variant Definition (Inline)
  let variantSuffix = null;
  if (
    node.aggregationData?.id?.level === AggregationLevel.MODULE &&
    node.aggregationData.id.id?.moduleVariant?.def
  ) {
    const def = node.aggregationData.id.id.moduleVariant.def;
    const text = getVariantDefinitionString(def);
    if (text) {
      variantSuffix = (
        <Tooltip title={text}>
          <Typography
            component="span"
            variant="caption"
            sx={{ color: 'text.secondary', ml: 0.5 }}
          >
            ({text})
          </Typography>
        </Tooltip>
      );
    }
  }

  return (
    <Box sx={{ display: 'flex', alignItems: 'flex-start', width: '100%' }}>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'flex-start',
          width: '24px',
          flexShrink: 0,
          color: 'action.active',
        }}
      >
        <IconButton size="small" onClick={handleToggle} tabIndex={-1}>
          {isExpanded ? (
            <ExpandMoreIcon fontSize="inherit" />
          ) : (
            <ChevronRightIcon fontSize="inherit" />
          )}
        </IconButton>
      </Box>

      <Box sx={{ flexGrow: 1 }}>
        <Typography
          variant="subtitle1"
          component="div"
          sx={{
            color: 'text.primary',
            fontSize: '0.9rem',
            fontWeight: 500,
            lineHeight: 1.4,
            mb: 0.5,
            wordBreak: 'break-all',
          }}
        >
          {label}
          {variantSuffix}
        </Typography>

        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 0.5,
            flexWrap: 'wrap',
          }}
        >
          {node.isLoading ? (
            <Skeleton
              variant="rectangular"
              width={120}
              height={24}
              sx={{ borderRadius: 1 }}
            />
          ) : (
            <>
              {semanticStatus !== 'unknown' && (
                <StatusChip
                  label={semanticStatus}
                  statusStyle={statusStyle}
                  semanticStatus={semanticStatus}
                />
              )}
              {summary && (
                <StatusChip
                  label={summary}
                  statusStyle={statusStyle}
                  semanticStatus={semanticStatus}
                />
              )}
            </>
          )}
        </Box>
      </Box>
    </Box>
  );
}

interface LeafTreeItemContentProps {
  node: LeafAggregationNode;
  rawInvocationId: string;
}

function LeafTreeItemContent({
  node,
  rawInvocationId,
}: LeafTreeItemContentProps) {
  const label = node.label || 'Unknown';
  const semanticStatus = getSemanticStatus(node);
  const statusStyle = getStatusStyle(semanticStatus);

  const linkUrl = node.verdict?.testIdStructured
    ? generateTestInvestigateUrl(rawInvocationId, node.verdict.testIdStructured)
    : undefined;

  const displayLabel = linkUrl ? (
    <MuiLink
      component={Link}
      to={linkUrl}
      underline="hover"
      color="inherit"
      onClick={(e) => e.stopPropagation()}
    >
      {label}
    </MuiLink>
  ) : (
    label
  );

  return (
    <Box sx={{ display: 'flex', alignItems: 'flex-start', width: '100%' }}>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'flex-start',
          width: '24px',
          flexShrink: 0,
          visibility: 'hidden',
          color: 'action.active',
        }}
      >
        <IconButton size="small" tabIndex={-1}>
          <ChevronRightIcon fontSize="inherit" />
        </IconButton>
      </Box>

      <Box sx={{ flexGrow: 1 }}>
        <Typography
          variant="subtitle1"
          component="div"
          sx={{
            color: 'text.primary',
            fontSize: '0.9rem',
            fontWeight: 500,
            lineHeight: 1.4,
            mb: 0.5,
            wordBreak: 'break-all',
          }}
        >
          {displayLabel}
        </Typography>

        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 0.5,
            flexWrap: 'wrap',
          }}
        >
          {semanticStatus && (
            <StatusChip
              label={semanticStatus}
              statusStyle={statusStyle}
              semanticStatus={semanticStatus}
            />
          )}
        </Box>
      </Box>
    </Box>
  );
}

interface AggregationTreeItemContentProps {
  node: AggregationNode;
  rawInvocationId: string;
  isExpanded: boolean;
  handleToggle: (e: React.MouseEvent) => void;
}

function AggregationTreeItemContent({
  node,
  rawInvocationId,
  isExpanded,
  handleToggle,
}: AggregationTreeItemContentProps) {
  if (node.isLeaf) {
    return (
      <LeafTreeItemContent node={node} rawInvocationId={rawInvocationId} />
    );
  }
  return (
    <IntermediateTreeItemContent
      node={node}
      isExpanded={isExpanded}
      handleToggle={handleToggle}
    />
  );
}
