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

import { Box, Divider, Typography } from '@mui/material';

import { checkKindToJSON } from '@/proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { checkStateToJSON } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';
import { CheckView } from '@/proto/turboci/graph/orchestrator/v1/check_view.pb';

import { DetailRow } from './detail_row';
import { GenericJsonDetails } from './generic_json_details';

export interface CheckDetailsProps {
  view: CheckView;
}

export function CheckDetails({ view }: CheckDetailsProps) {
  const check = view.check;
  if (!check) return null;

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

      {check.options.length > 0 && <Divider />}

      {check.options.length > 0 && (
        <>
          <Typography variant="subtitle2">Options</Typography>
          {view.optionData.map((datum, idx) => {
            // TODO - We should be able to get the typeUrl directly from the view
            // in the updated protos.
            return (
              <GenericJsonDetails
                key={`check-option-${idx}`}
                json={datum.value?.valueJson}
              />
            );
          })}
        </>
      )}

      {check.results.length > 0 && <Divider />}

      {check.results.length > 0 && (
        <>
          <Typography variant="subtitle2">Results</Typography>
          {view.results.map((resultView) => {
            // TODO - We should be able to get the typeUrl directly from the view
            // in the updated protos.
            if (resultView.data.length === 0) {
              return null;
            }
            return resultView.data.map((datum, idx) => (
              <GenericJsonDetails
                key={`check-result-${idx}`}
                json={datum.value?.valueJson}
              />
            ));
          })}
        </>
      )}
    </Box>
  );
}
