// Copyright 2022 The LUCI Authors.
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

import dayjs from 'dayjs';
import {
  ReactNode,
  useState,
} from 'react';

import { styled } from '@mui/material/styles';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import Chip from '@mui/material/Chip';
import IconButton from '@mui/material/IconButton';
import Link from '@mui/material/Link';
import TableCell, { tableCellClasses } from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import DoneIcon from '@mui/icons-material/Done';
import RampLeftIcon from '@mui/icons-material/RampLeft';
import CloseIcon from '@mui/icons-material/Close';
import RemoveIcon from '@mui/icons-material/Remove';
import HistoryIcon from '@mui/icons-material/History';

import { Tooltip } from '@mui/material';
import {
  DistinctClusterFailure,
  DistinctClusterFailure_PresubmitRun,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import {
  FailureGroup,
} from '@/tools/failures_tools';
import {
  invocationName,
  failureLink,
  testHistoryLink,
  presubmitRunLink,
} from '@/tools/urlHandling/links';

import CLList from '@/components/cl_list/cl_list';
import { PresubmitRunMode, PresubmitRunStatus } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';

interface Props {
  project: string;
  group: FailureGroup;
  selectedVariantGroups: string[];
  children?: ReactNode;
}

interface VariantPair {
  key: string;
  value: string;
}

const NarrowTableCell = styled(TableCell)(() => ({
  [`&.${tableCellClasses.root}`]: {
    padding: '6px 6px',
  },
}));

const FailuresTableRows = ({
  project,
  group,
  selectedVariantGroups,
  children = null,
}: Props) => {
  const [expanded, setExpanded] = useState(false);

  const toggleExpand = () => {
    setExpanded(!expanded);
  };

  const ungroupedVariants = (failure: DistinctClusterFailure): VariantPair[] => {
    const def = failure.variant?.def;
    const unselectedVariantPairs: VariantPair[] = [];
    for (const key in def) {
      if (!Object.prototype.hasOwnProperty.call(def, key)) {
        continue;
      }
      if (selectedVariantGroups.includes(key)) {
        continue;
      }
      const value = def[key] || '';
      unselectedVariantPairs.push({ key: key, value: value });
    }
    return unselectedVariantPairs;
  };

  const presubmitRunIcon = (run: DistinctClusterFailure_PresubmitRun) => {
    if (run.status == PresubmitRunStatus.SUCCEEDED) {
      if (run.mode == PresubmitRunMode.FULL_RUN) {
        return <RampLeftIcon />;
      } else {
        return <DoneIcon />;
      }
    } else if (run.status == PresubmitRunStatus.FAILED) {
      return <CloseIcon />;
    } else {
      return <RemoveIcon />;
    }
  };

  const presubmitRunLabel = (run: DistinctClusterFailure_PresubmitRun): string => {
    if (run.status == PresubmitRunStatus.SUCCEEDED) {
      if (run.mode == PresubmitRunMode.FULL_RUN) {
        return 'Submitted';
      } else {
        return 'Succeeded';
      }
    } else if (run.status == PresubmitRunStatus.FAILED) {
      return 'Failed';
    } else {
      return 'Canceled';
    }
  };

  const verdictLabel = (failure: DistinctClusterFailure): string => {
    if (failure.exonerations && failure.exonerations.length > 0) {
      return 'Exonerated';
    }
    if (failure.isIngestedInvocationBlocked) {
      return 'Unexpected';
    }
    return 'Flaky';
  };

  const verdictColor = (failure: DistinctClusterFailure) => {
    if (failure.exonerations && failure.exonerations.length > 0) {
      return 'info';
    }
    if (failure.isIngestedInvocationBlocked) {
      return 'error';
    }
    return 'warning';
  };

  return (
    <>
      <TableRow hover={true}
        style={{ cursor: group.failure ? undefined : 'pointer' }}
        onClick={() => !group.failure && toggleExpand()}>
        {group.failure ? (
          <>
            <NarrowTableCell
              sx={{
                padding: '0px',
                width: `${20 * group.level}px`,
              }}
              data-testid="failures_table_group_cell"
            >
            </NarrowTableCell>
            <NarrowTableCell
              data-testid="failures_table_build_cell"
            >
              <Link
                aria-label="Failure invocation id"
                sx={{ mr: 2 }}
                href={failureLink(group.failure.ingestedInvocationId, group.failure.testId, group.failure.variant)}
                target="_blank"
              >
                {invocationName(group.failure.ingestedInvocationId)}
              </Link>
            </NarrowTableCell>
            <NarrowTableCell
              data-testid="failures_table_verdict_cell"
            >
              <Chip
                label={verdictLabel(group.failure)}
                color={verdictColor(group.failure)}
                size='small'
                variant='outlined'
              />
            </NarrowTableCell>
            <NarrowTableCell
              data-testid="failures_table_variant_cell"
            >
              <small>{group.failure.failureReasonPrefix}{group.failure.failureReasonPrefix.length >= 120 ? '...' : ''}</small>
              <br />
              <small style={{ color: '#888' }}data-testid="ungrouped_variants">
                {ungroupedVariants(group.failure)
                    .map((v) => v && `${v.key}: ${v.value}`)
                    .filter((v) => v)
                    .join(', ')}
              </small>
            </NarrowTableCell>
            <NarrowTableCell
              data-testid="failures_table_cls_cell"
            >
              <CLList changelists={group.failure?.changelists || []} />
            </NarrowTableCell>
            <NarrowTableCell
              data-testid="failures_table_presubmit_run_cell"
            >
              {group.failure.presubmitRun && (
                <Chip
                  icon={presubmitRunIcon(group.failure.presubmitRun)}
                  label={presubmitRunLabel(group.failure.presubmitRun)}
                  color='default'
                  size='small'
                  component='a'
                  target='_blank'
                  variant='outlined'
                  clickable
                  // Will always be non-null if presubmitRun is non-null.
                  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
                  href={presubmitRunLink(group.failure.presubmitRun.presubmitRunId!)}
                />
              )}
            </NarrowTableCell>
          </>
        ) : (
          <NarrowTableCell
            key={group.id}
            data-testid="failures_table_group_cell"
            colSpan={6}
          >
            <div style={{
              display: 'flex',
              alignItems: 'center',
              flexWrap: 'nowrap',
              width: '100%',
            }}>
              <div style={{ flex: '0 0 auto', width: `${20 * group.level}px`, height: '20px' }}></div>
              <div style={{ flex: '0 0 auto', width: '40px' }}>
                <IconButton
                  aria-label="Expand group"
                >
                  {expanded ? <ExpandMoreIcon /> : <ChevronRightIcon />}
                </IconButton>
              </div>
              {/** Place test name or variant value in a separate span to allow better testability */}
              <div style={{ flex: '0 0 auto' }}>
                <Chip label={group.key.key || 'Test'} sx={{ cursor: 'pointer' }} />&nbsp;
              </div>
              <div style={{ flex: '1 1 auto', wordBreak: 'break-word' }}>
                {group.key.value || 'none'}
              </div>
              <div style={{ flex: '0 0 auto', width: '40px', textAlign: 'center' }}>
                {group.key.type == 'test' ?
                  <Tooltip title={<><b>Test History</b><br />View all recent results of this test including passes and failures in other clusters. Results are filtered by the selected &lsquo;Group By&rsquo; fields.</>}>
                    <Link
                      aria-label='Test history link'
                      href={testHistoryLink(project, group.key.value, group.commonVariant)}
                      onClick={(e) => e.stopPropagation() /* prevent toggling group expansion */ }
                      target="_blank">
                      <HistoryIcon />
                    </Link>
                  </Tooltip> : <>&nbsp;</>}
              </div>
            </div>
          </NarrowTableCell>
        )}
        <NarrowTableCell data-testid="failure_table_group_presubmitrejects">
          {group.failure ? (
            <>
              {group.failure.presubmitRun ? (
                group.presubmitRejects
              ) : (
                '-'
              )}
            </>
          ) : (
            group.presubmitRejects
          )}
        </NarrowTableCell>
        <NarrowTableCell className="number">
          {group.invocationFailures}
        </NarrowTableCell>
        <NarrowTableCell className="number">
          {group.criticalFailuresExonerated}
        </NarrowTableCell>
        <NarrowTableCell className="number">
          {group.failures}
        </NarrowTableCell>
        <NarrowTableCell>
          {dayjs(group.latestFailureTime).fromNow()}
        </NarrowTableCell>
      </TableRow>
      {/** Render the remaining rows in the group */}
      {expanded && children}
    </>
  );
};

export default FailuresTableRows;
