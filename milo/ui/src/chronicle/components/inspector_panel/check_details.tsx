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

import { Box, Chip, Divider, Typography } from '@mui/material';

import { checkKindToJSON } from '@/proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { checkStateToJSON } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';
import { CheckView } from '@/proto/turboci/graph/orchestrator/v1/check_view.pb';

import { AnyDetails } from './any_details';
import { DetailRow } from './detail_row';

export interface CheckDetailsProps {
  view: CheckView;
}

export function CheckDetails({ view }: CheckDetailsProps) {
  const check = view.check;
  if (!check) return null;

  const dependencyIds = (check.dependencies?.edges || [])
    .map((edge) => edge.check?.identifier?.id || edge.stage?.identifier?.id)
    .filter((id): id is string => !!id)
    .sort();

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
        <DetailRow label="ID" value={check.identifier?.id} />
        <DetailRow
          label="Kind"
          value={check.kind ? checkKindToJSON(check.kind) : 'UNKNOWN'}
        />
        <DetailRow
          label="State"
          value={check.state ? checkStateToJSON(check.state) : 'UNKNOWN'}
        />
        <DetailRow label="Realm" value={check.realm} />
        <DetailRow label="Last Updated" value={check.version?.ts} />
      </Box>

      {dependencyIds.length > 0 && (
        <DetailRow
          label="Dependencies"
          value={dependencyIds.map((id) => (
            <Chip key={id} label={id} size="small" />
          ))}
        />
      )}

      {check.options.length > 0 && <Divider />}

      {check.options.length > 0 && (
        <>
          <Typography variant="subtitle2">Options</Typography>
          {Object.entries(view.optionData).map(([typeUrl, datum]) => (
            <AnyDetails
              key={`check-option-${typeUrl}`}
              typeUrl={typeUrl}
              json={datum.value?.valueJson}
            />
          ))}
        </>
      )}

      {check.results.length > 0 && <Divider />}

      {check.results.length > 0 && (
        <>
          <Typography variant="subtitle2">Results</Typography>
          {Object.entries(view.results).map(([resIdx, resultView]) => {
            const dataEntries = Object.entries(resultView.data);
            if (dataEntries.length === 0) return null;

            return dataEntries.map(([typeUrl, datum]) => (
              <AnyDetails
                key={`result-${resIdx}-${typeUrl}`}
                typeUrl={typeUrl}
                json={datum.value?.valueJson}
              />
            ));
          })}
        </>
      )}
    </Box>
  );
}
