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
import Grid from '@mui/material/Grid';
import IconButton from '@mui/material/IconButton';
import Link from '@mui/material/Link';
import TableCell, { tableCellClasses } from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import DoneIcon from '@mui/icons-material/Done';
import RampLeftIcon from '@mui/icons-material/RampLeft';
import CloseIcon from '@mui/icons-material/Close';
import RemoveIcon from '@mui/icons-material/Remove';

import {
  DistinctClusterFailure,
  PresubmitRun,
} from '@/services/cluster';
import {
  FailureGroup,
  GroupKey,
  VariantGroup,
} from '@/tools/failures_tools';
import {
  invocationName,
  failureLink,
  testHistoryLink,
  presubmitRunLink,
} from '@/tools/urlHandling/links';

import CLList from '@/components/cl_list/cl_list';
import { Variant } from '@/services/shared_models';

interface Props {
  project: string;
  parentKeys?: GroupKey[];
  group: FailureGroup;
  variantGroups: VariantGroup[];
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
  parentKeys = [],
  group,
  variantGroups,
  children = null,
}: Props) => {
  const [expanded, setExpanded] = useState(false);

  const toggleExpand = () => {
    setExpanded(!expanded);
  };

  const ungroupedVariants = (failure: DistinctClusterFailure): VariantPair[] => {
    const unselectedVariants = variantGroups
        .filter((v) => !v.isSelected)
        .map((v) => v.key);
    const unselectedVariantPairs: (VariantPair|null)[] =
      unselectedVariants.map((key) => {
        const value = failure.variant?.def[key];
        if (value !== undefined) {
          return { key: key, value: value };
        }
        return null;
      });
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    return unselectedVariantPairs.filter((vp) => vp != null).map((vp) => vp!);
  };

  const groupByVariant = (): Variant => {
    // Returns the parent grouping keys as a partial variant.
    const result: Variant = { def: {} };
    parentKeys.forEach((v) => {
      if (v.type == 'variant' && v.key) {
        result.def[v.key] = v.value;
      }
    });
    return result;
  };

  const presubmitRunIcon = (run: PresubmitRun) => {
    if (run.status == 'PRESUBMIT_RUN_STATUS_SUCCEEDED') {
      if (run.mode == 'FULL_RUN') {
        return <RampLeftIcon/>;
      } else {
        return <DoneIcon/>;
      }
    } else if (run.status == 'PRESUBMIT_RUN_STATUS_FAILED') {
      return <CloseIcon/>;
    } else {
      return <RemoveIcon/>;
    }
  };

  const presubmitRunLabel = (run: PresubmitRun): string => {
    if (run.status == 'PRESUBMIT_RUN_STATUS_SUCCEEDED') {
      if (run.mode == 'FULL_RUN') {
        return 'Submitted';
      } else {
        return 'Succeeded';
      }
    } else if (run.status == 'PRESUBMIT_RUN_STATUS_FAILED') {
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
      <TableRow hover={true}>
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
                href={failureLink(group.failure.ingestedInvocationId, group.failure.testId)}
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
              <small data-testid="ungrouped_variants">
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
                  href={presubmitRunLink(group.failure.presubmitRun.presubmitRunId)}
                />
              )}
            </NarrowTableCell>
          </>
          ) : (
          <NarrowTableCell
            key={group.id}
            sx={{
              paddingLeft: `${20 * group.level}px`,
              width: '60%',
            }}
            data-testid="failures_table_group_cell"
            colSpan={6}
          >
            <Grid
              container
              justifyContent="start"
              alignItems="baseline"
              columnGap={2}
              flexWrap="nowrap"
            >
              <Grid item>
                <IconButton
                  aria-label="Expand group"
                  onClick={() => toggleExpand()}
                >
                  {expanded ? <ExpandMoreIcon /> : <ChevronRightIcon />}
                </IconButton>
              </Grid>
              <Grid item sx={{ overflowWrap: 'anywhere' }}>
                {/** Place test name or variant value in a separate span to allow better testability */}
                <span>{group.key.value || 'none'}</span>
                {group.key.type == 'test' ? (
                <>
                  &nbsp;-&nbsp;
                  <Link
                    sx={{ display: 'inline-flex' }}
                    aria-label='Test history link'
                    href={testHistoryLink(project, group.key.value, groupByVariant())}
                    target="_blank">
                      History
                  </Link>
                </>) : null}
              </Grid>
            </Grid>
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
        <NarrowTableCell className="number">{group.invocationFailures}</NarrowTableCell>
        <NarrowTableCell className="number">{group.criticalFailuresExonerated}</NarrowTableCell>
        <NarrowTableCell className="number">{group.failures}</NarrowTableCell>
        <NarrowTableCell>{dayjs(group.latestFailureTime).fromNow()}</NarrowTableCell>
      </TableRow>
      {/** Render the remaining rows in the group */}
      {expanded && children}
    </>
  );
};

export default FailuresTableRows;
