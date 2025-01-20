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

import Typography from '@mui/material/Typography';
import { DateTime } from 'luxon';

import HelpTooltip from '@/clusters/components/help_tooltip/help_tooltip';
import { Problem } from '@/clusters/tools/problems';
import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { SanitizedHtml } from '@/common/components/sanitized_html';
import { displayApproxDuartion } from '@/common/tools/time_utils';

import { PriorityChip } from '../priority_chip/priority_chip';
import { StatusChip } from '../status_chip/status_chip';

import { CriteriaSection } from './criteria_section/criteria_section';

const problemDescriptionTooltipText =
  'Information about the problem and why it should be fixed. Supplied by the policy owner.';
const actionDescriptionTooltipText =
  'Suggested actions for resolving the problem. Supplied by the policy owner.';
const reactivationCriteriaTooltipText =
  'The condition that, if met, that will cause LUCI Analysis to re-open this problem. This is configured by the policy owner.';
const resolutionCriteriaTooltipText =
  'The condition that must be met for this problem to be considered resolved.' +
  ' This is configured by the policy owner. Once all problems are resolved,' +
  ' LUCI Analysis will automatically close the associated bug (if the Update Bug option is enabled on the rule).';
const policyOwnersTooltipText =
  'The contacts from your project which own the policy which identified this problem.';
const activeSinceTooltipText =
  'When this problem started (when the policy activation criteria was met).';
const resolvedSinceTooltipText =
  'When this problem stopped (when the policy resolution criteria was met).';

export interface Props {
  problem: Problem;
}

export const ProblemExplanationSection = ({ problem }: Props) => {
  return (
    <>
      <Typography component="div" gutterBottom>
        <StatusChip isActive={problem.state.isActive} />
        {problem.state.isActive && (
          <>
            &nbsp;
            <PriorityChip priority={problem.policy.priority} />
          </>
        )}
      </Typography>
      <Typography
        component="h6"
        paddingTop="0.5rem"
        sx={{ fontWeight: 'bold' }}
      >
        Problem description
        <HelpTooltip text={problemDescriptionTooltipText} />
      </Typography>
      <Typography component="div" gutterBottom>
        {/* Explanation field is always set on policies. This is enforced by config validation. */}
        {/* eslint-disable-next-line @typescript-eslint/no-non-null-assertion */}
        <SanitizedHtml html={problem.policy.explanation!.problemHtml} />
      </Typography>
      <Typography
        component="h6"
        paddingTop="0.5rem"
        sx={{ fontWeight: 'bold' }}
      >
        How to resolve
        <HelpTooltip text={actionDescriptionTooltipText} />
      </Typography>
      <Typography component="div" gutterBottom>
        {/* Explanation field is always set on policies. This is enforced by config validation. */}
        {/* eslint-disable-next-line @typescript-eslint/no-non-null-assertion */}
        <SanitizedHtml html={problem.policy.explanation!.actionHtml} />
      </Typography>
      {problem.state.isActive && (
        <>
          <Typography
            component="h6"
            paddingTop="0.5rem"
            sx={{ fontWeight: 'bold' }}
          >
            Resolution Criteria
            <HelpTooltip text={resolutionCriteriaTooltipText} />
          </Typography>
          <Typography component="div" gutterBottom>
            <CriteriaSection
              policy={problem.policy}
              showActivationCriteria={false}
            />
          </Typography>
          <Typography
            component="h6"
            paddingTop="0.5rem"
            sx={{ fontWeight: 'bold' }}
          >
            Active since
            <HelpTooltip text={activeSinceTooltipText} />
          </Typography>
          <Typography gutterBottom>
            <RelativeTimestamp
              formatFn={displayApproxDuartion}
              timestamp={DateTime.fromISO(
                problem.state.lastActivationTime || '',
              )}
            />
          </Typography>
        </>
      )}
      {!problem.state.isActive && (
        <>
          <Typography
            component="h6"
            paddingTop="0.5rem"
            sx={{ fontWeight: 'bold' }}
          >
            Re-activation Criteria
            <HelpTooltip text={reactivationCriteriaTooltipText} />
          </Typography>
          <Typography component="div" gutterBottom>
            <CriteriaSection
              policy={problem.policy}
              showActivationCriteria={true}
            />
          </Typography>
          <Typography
            component="h6"
            paddingTop="0.5rem"
            sx={{ fontWeight: 'bold' }}
          >
            Resolved since
            <HelpTooltip text={resolvedSinceTooltipText} />
          </Typography>
          <Typography gutterBottom>
            <RelativeTimestamp
              formatFn={displayApproxDuartion}
              timestamp={DateTime.fromISO(
                problem.state.lastDeactivationTime || '',
              )}
            />
          </Typography>
        </>
      )}
      <Typography
        component="h6"
        paddingTop="0.5rem"
        sx={{ fontWeight: 'bold' }}
      >
        Policy owner(s)
        <HelpTooltip text={policyOwnersTooltipText} />
      </Typography>
      <Typography>{problem.policy.owners.join(', ')}</Typography>
    </>
  );
};
