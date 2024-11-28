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

import { TableCell, TableRow } from '@mui/material';

import { LuciBisectionResultSection } from '@/monitoringv2/components/luci_bisection_result';
import { ReasonSection } from '@/monitoringv2/components/reason_section';
import { AlertJson, Bug, TreeJson } from '@/monitoringv2/util/server_json';

interface AlertDetailsRowProps {
  tree: TreeJson;
  alert: AlertJson;
  bug?: Bug;
}

/**
 * A row in the AlertTable showing the details for an alert.  These will only
 * be displayed if the alert is expanded.  They are always shown directly below
 * the AlertSummaryRow for the same alert.
 */
export const AlertDetailsRow = ({ alert, tree, bug }: AlertDetailsRowProps) => {
  return (
    <TableRow sx={{ backgroundColor: 'var(--block-background-color)' }}>
      <TableCell></TableCell>
      <TableCell colSpan={100}>
        <div css={{ marginBottom: '10px', backgroundColor: '#fff' }}>
          <ReasonSection
            builder={alert.extension.builders[0]}
            tree={tree}
            reason={alert.extension.reason}
            failureBuildUrl={alert.extension.builders[0].latest_failure_url}
            bug={bug}
          />
          {alert.extension.luci_bisection_result ? (
            <LuciBisectionResultSection
              result={alert.extension.luci_bisection_result}
            />
          ) : null}
        </div>
      </TableCell>
    </TableRow>
  );
};
