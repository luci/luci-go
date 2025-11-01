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

import { stageStateToJSON } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';
import { StageView } from '@/proto/turboci/graph/orchestrator/v1/stage_view.pb';

import { AnyDetails } from './any_details';
import { DetailRow } from './detail_row';

export interface StageDetailsProps {
  view: StageView;
}

export function StageDetails({ view }: StageDetailsProps) {
  const stage = view.stage;
  if (!stage) return null;

  const assignmentIds = stage.assignments
    .map((a) => a.target?.id)
    .filter((id): id is string => !!id)
    .sort();

  const dependencyIds = (stage.dependencies?.edges || [])
    .map((edge) => edge.target?.check?.id || edge.target?.stage?.id)
    .filter((id): id is string => !!id)
    .sort();

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
        <DetailRow label="ID" value={stage.identifier?.id} />
        <DetailRow
          label="State"
          value={stage.state ? stageStateToJSON(stage.state) : 'UNKNOWN'}
        />
        <DetailRow label="Realm" value={stage.realm} />
        <DetailRow label="Created" value={stage.createTs?.ts} />
        <DetailRow label="Last Updated" value={stage.version?.ts} />
      </Box>

      {dependencyIds.length > 0 && (
        <DetailRow
          label="Dependencies"
          value={dependencyIds.map((id) => (
            <Chip key={id} label={id} size="small" />
          ))}
        />
      )}

      {assignmentIds.length > 0 && (
        <DetailRow
          label="Assignments"
          value={assignmentIds.map((id) => (
            <Chip key={id} label={id} size="small" />
          ))}
        />
      )}

      {stage.args?.valueJson && (
        <>
          <Divider />
          <Typography variant="subtitle2">Args</Typography>
          <AnyDetails json={stage.args.valueJson} />
        </>
      )}
    </Box>
  );
}
