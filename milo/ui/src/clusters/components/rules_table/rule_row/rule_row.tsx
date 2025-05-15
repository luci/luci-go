// Copyright 2023 The LUCI Authors.
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

import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import Tooltip from '@mui/material/Tooltip';
import { DateTime } from 'luxon';
import { Link as RouterLink } from 'react-router';

import {
  identifyProblems,
  sortProblemsByDescendingActiveAndPriority,
} from '@/clusters/tools/problems';
import { linkToRule } from '@/clusters/tools/urlHandling/links';
import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { displayApproxDuartion } from '@/common/tools/time_utils';
import { BugManagement } from '@/proto/go.chromium.org/luci/analysis/proto/v1/projects.pb';
import { Rule } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';

import ProblemChip from '../problem_chip/problem_chip';

interface RowProps {
  rule: Rule;
  bugManagementConfig: BugManagement | undefined;
  focusPolicyID: string;
  colorIndexFunc: (policyID: string) => number;
}

const RuleRow = ({
  rule,
  bugManagementConfig,
  focusPolicyID,
  colorIndexFunc,
}: RowProps) => {
  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
  const problems = identifyProblems(
    bugManagementConfig,
    rule.bugManagementState!,
  );
  sortProblemsByDescendingActiveAndPriority(problems, focusPolicyID);

  return (
    <TableRow>
      <TableCell>
        <Link
          component={RouterLink}
          to={linkToRule(rule.project, rule.ruleId)}
          underline="hover"
        >
          {rule.ruleDefinition || 'Click to see example failures.'}
        </Link>
      </TableCell>
      <TableCell>
        {problems.map((p) => (
          <Tooltip
            key={p.policy.id}
            placement="left"
            arrow
            title={
              <>
                <strong>Problem: {p.policy.humanReadableName}</strong>
                <br />
                {p.state.isActive ? (
                  <>
                    Active since{' '}
                    <RelativeTimestamp
                      formatFn={displayApproxDuartion}
                      timestamp={DateTime.fromISO(
                        p.state.lastActivationTime || '',
                      )}
                    />
                  </>
                ) : (
                  <>
                    Resolved since{' '}
                    <RelativeTimestamp
                      formatFn={displayApproxDuartion}
                      timestamp={DateTime.fromISO(
                        p.state.lastDeactivationTime || '',
                      )}
                    />
                  </>
                )}
              </>
            }
          >
            <Box>
              <ProblemChip
                policy={p.policy}
                // Fade out if we are focusing a policy and we are not that policy.
                fadedOut={focusPolicyID !== '' && p.policy.id !== focusPolicyID}
                active={p.state.isActive}
                colorIndex={colorIndexFunc(p.policy.id)}
              />
            </Box>
          </Tooltip>
        ))}
      </TableCell>
      {/* eslint-disable-next-line @typescript-eslint/no-non-null-assertion */}
      <TableCell>
        <Link href={rule.bug!.url} underline="hover">
          {rule.bug!.linkText}
        </Link>
      </TableCell>
      <TableCell>
        <RelativeTimestamp
          formatFn={displayApproxDuartion}
          timestamp={DateTime.fromISO(rule.lastAuditableUpdateTime || '')}
        />
      </TableCell>
    </TableRow>
  );
};

export default RuleRow;
