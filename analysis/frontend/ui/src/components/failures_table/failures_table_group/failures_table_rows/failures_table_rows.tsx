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

import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import ArrowRightIcon from '@mui/icons-material/ArrowRight';
import Grid from '@mui/material/Grid';
import IconButton from '@mui/material/IconButton';
import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';

import { DistinctClusterFailure } from '@/services/cluster';
import {
  FailureGroup,
  GroupKey,
  VariantGroup,
} from '@/tools/failures_tools';
import {
  failureLink,
  testHistoryLink,
} from '@/tools/urlHandling/links';

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

  const query = parentKeys.filter((v) => v.type == 'variant').map((v) => {
    return 'V:' + encodeURIComponent(v.key || '') + '=' + encodeURIComponent(v.value);
  }).join(' ');

  return (
    <>
      <TableRow>
        <TableCell
          key={group.id}
          sx={{
            paddingLeft: `${20 * group.level}px`,
            width: '60%',
          }}
          data-testid="failures_table_group_cell"
        >
          {group.failure ? (
            <>
              <Link
                aria-label="Failure invocation id"
                sx={{ mr: 2 }}
                href={failureLink(group.failure)}
                target="_blank"
              >
                {group.failure.ingestedInvocationId}
              </Link>
              <small data-testid="ungrouped_variants">
                {ungroupedVariants(group.failure)
                    .map((v) => v && `${v.key}: ${v.value}`)
                    .filter((v) => v)
                    .join(', ')}
              </small>
            </>
          ) : (
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
                  {expanded ? <ArrowDropDownIcon /> : <ArrowRightIcon />}
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
                    href={testHistoryLink(project, group.key.value, query)}
                    underline='hover'
                    target="_blank">
                      History
                  </Link>
                </>) : null}
              </Grid>
            </Grid>
          )}
        </TableCell>
        <TableCell data-testid="failure_table_group_presubmitrejects">
          {group.failure ? (
            <>
              {group.failure.presubmitRun ? (
                <Link
                  aria-label="Presubmit rejects link"
                  href={`https://luci-change-verifier.appspot.com/ui/run/${group.failure.presubmitRun.presubmitRunId.id}`}
                  target="_blank"
                >
                  {group.presubmitRejects}
                </Link>
              ) : (
                '-'
              )}
            </>
          ) : (
            group.presubmitRejects
          )}
        </TableCell>
        <TableCell className="number">{group.invocationFailures}</TableCell>
        <TableCell className="number">{group.criticalFailuresExonerated}</TableCell>
        <TableCell className="number">{group.failures}</TableCell>
        <TableCell>{dayjs(group.latestFailureTime).fromNow()}</TableCell>
      </TableRow>
      {/** Render the remaining rows in the group */}
      {expanded && children}
    </>
  );
};

export default FailuresTableRows;
