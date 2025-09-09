// Copyright 2024 The LUCI Authors.
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
import { Checkbox, IconButton, TableCell, TableRow } from '@mui/material';
import { Link } from '@mui/material';

import { useFeatureFlag } from '@/common/feature_flags';
import { SHOW_GEN_AI_SUSPECTS } from '@/monitoringv2/pages/monitoring_page/features';
import { GenericAlert, StructuredAlert } from '@/monitoringv2/util/alerts';

import { TestHistorySparkline } from '../test_history_sparkline';

import { PrefillFilterIcon } from './prefill_filter_icon';
import { TruncatedHistoryExplanation } from './truncated_history_explanation';

interface AlertTestRowProps {
  parentAlert?: GenericAlert;
  alert: StructuredAlert;
  expanded: boolean;
  indent: number;
  onExpand: () => void;
  selected: boolean;
  toggleSelected: () => void;
}

/** An expandable row in the AlertTable containing a summary of a single alert. */
export const TestAlertRow = ({
  parentAlert,
  alert,
  onExpand,
  expanded,
  indent,
  selected,
  toggleSelected,
}: AlertTestRowProps) => {
  const testAlert = alert.alert;
  const consecutiveFailures = testAlert.consecutiveFailures;
  const firstFailureId = testAlert.history[consecutiveFailures - 1]?.buildId;
  const showGenAiSuspects: boolean = useFeatureFlag(SHOW_GEN_AI_SUSPECTS);
  if (testAlert.kind !== 'test') {
    throw new Error(
      `TestAlertRow can only be used with test alerts, not ${testAlert.kind}`,
    );
  }

  return (
    <TableRow
      hover
      sx={{
        cursor: 'pointer',
        opacity:
          consecutiveFailures === 0 ||
          (parentAlert &&
            testAlert.consecutiveFailures > parentAlert.consecutiveFailures)
            ? '0.5'
            : '1',
      }}
    >
      <TableCell width="32px" padding="none">
        <Checkbox
          sx={{ marginLeft: `${indent * 20}px` }}
          checked={selected}
          onChange={toggleSelected}
        />
      </TableCell>
      <TableCell
        width="32px"
        padding="none"
        onClick={() => onExpand()}
        title={expanded ? 'Collapse' : 'Expand'}
      >
        {alert.children.length > 0 && (
          <IconButton sx={{ marginLeft: `${indent * 20}px` }}>
            {expanded ? <ExpandMoreIcon /> : <ChevronRightIcon />}
          </IconButton>
        )}
      </TableCell>
      <TableCell>
        <span
          style={{
            paddingLeft: `${indent * 20}px`,
            opacity: '75%',
            fontSize: '90%',
            fontWeight: 300,
          }}
        >
          Test:
        </span>{' '}
        {testAlert.testName}
        <PrefillFilterIcon filter={testAlert.testName} />
      </TableCell>
      <TableCell width="180px">
        <TestHistorySparkline
          project={testAlert.builderID.project}
          testId={testAlert.testId}
          variantHash={testAlert.variantHash}
          history={testAlert.history}
          numHighlighted={consecutiveFailures}
        />
      </TableCell>
      {showGenAiSuspects ? (
        <TableCell width="200px"></TableCell>
      ) : (
        <TableCell width="120px">
          {consecutiveFailures > 0 &&
            consecutiveFailures < testAlert.history.length && (
              <Link
                href={`/b/${firstFailureId}`}
                target="_blank"
                rel="noreferrer"
                onClick={(e) => e.stopPropagation()}
              >
                {consecutiveFailures} build
                {consecutiveFailures > 1 && 's'} ago
              </Link>
            )}
          {consecutiveFailures === testAlert.history.length && (
            <Link
              href={`/ui/p/${testAlert.builderID.project}/builders/${testAlert.builderID.bucket}/${testAlert.builderID.builder}`}
              target="_blank"
              rel="noreferrer"
              onClick={(e) => e.stopPropagation()}
            >
              See history <TruncatedHistoryExplanation />
            </Link>
          )}
        </TableCell>
      )}
      <TableCell width="100px">
        {firstFailureId && consecutiveFailures < testAlert.history.length && (
          <Link
            href={`/b/${firstFailureId}/blamelist`}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
          >
            Blamelist
          </Link>
        )}
        {consecutiveFailures === testAlert.history.length && (
          <Link
            href={`/ui/p/${testAlert.builderID.project}/builders/${testAlert.builderID.bucket}/${testAlert.builderID.builder}`}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
          >
            See history <TruncatedHistoryExplanation />
          </Link>
        )}
      </TableCell>
    </TableRow>
  );
};
