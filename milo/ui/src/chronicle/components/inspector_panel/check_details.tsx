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
import { useContext, useMemo } from 'react';

import {
  AssignmentType,
  getStageAssignments,
} from '@/chronicle/utils/assignment_utils';
import { toString } from '@/chronicle/utils/id';
import { renderTimestamp } from '@/chronicle/utils/time_utils';
import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { checkKindToJSON } from '@/proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { checkStateToJSON } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { ChronicleContext } from '../context';

import { AnyDetails } from './any_details';
import { DetailRow } from './detail_row';
import { GenericJsonDetails } from './generic_json_details';
import { RenderMode } from './types';

export interface CheckDetailsProps {
  check: Check;
  valueDataMap: Map<string, ValueData>;
  renderMode?: RenderMode;
}

export function CheckDetails({
  check,
  valueDataMap,
  renderMode,
}: CheckDetailsProps) {
  const { graph } = useContext(ChronicleContext);

  const dependencyIds = (check.dependencies?.edges || [])
    .map((edge) => {
      const idObj = edge.check?.identifier || edge.stage?.identifier;
      return idObj ? toString(idObj) : '';
    })
    .filter((id) => !!id)
    .sort();

  // Stages that work on this check (reverse of the stage's assignments).
  const checkId = check.identifier?.id;
  const { plannedBy, executedBy } = useMemo(() => {
    const plannedBy: string[] = [];
    const executedBy: string[] = [];
    if (checkId && graph) {
      for (const stage of graph.stages) {
        if (!stage.identifier?.id) continue;
        const assignment = getStageAssignments(stage).find(
          (a) => a.checkId === checkId,
        );
        if (!assignment) continue;
        const stageIdStr = toString(stage.identifier);
        if (assignment.type === AssignmentType.EDITING) {
          plannedBy.push(stageIdStr);
        } else if (assignment.type === AssignmentType.EXECUTION) {
          executedBy.push(stageIdStr);
        }
      }
    }
    return { plannedBy: plannedBy.sort(), executedBy: executedBy.sort() };
  }, [graph, checkId]);

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
      {renderMode === RenderMode.Json ? (
        <GenericJsonDetails json={JSON.stringify(check)} maxHeight="none" />
      ) : (
        <>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            <DetailRow label="ID" value={toString(check.identifier)} />
            <DetailRow
              label="Kind"
              value={check.kind ? checkKindToJSON(check.kind) : 'UNKNOWN'}
            />
            <DetailRow
              label="State"
              value={check.state ? checkStateToJSON(check.state) : 'UNKNOWN'}
            />
            <DetailRow label="Realm" value={check.realm} />
            <DetailRow
              label="Last Updated"
              value={renderTimestamp(check.version?.ts)}
            />
          </Box>

          {dependencyIds.length > 0 && (
            <DetailRow
              label="Dependencies"
              value={dependencyIds.map((id) => (
                <Chip key={id} label={id} size="small" />
              ))}
            />
          )}

          {plannedBy.length > 0 && (
            <DetailRow
              label="Planned by"
              value={plannedBy.map((id) => (
                <Chip key={id} label={id} size="small" />
              ))}
            />
          )}

          {executedBy.length > 0 && (
            <DetailRow
              label="Executed by"
              value={executedBy.map((id) => (
                <Chip key={id} label={id} size="small" />
              ))}
            />
          )}
        </>
      )}

      {check.options?.length > 0 && (
        <>
          <Divider />
          <Typography variant="subtitle2">Options</Typography>
          {check.options.map((value_ref, index) => (
            <AnyDetails
              key={`check-option-${index}`}
              typeUrl={value_ref.typeUrl}
              omitReason={value_ref.omitReason}
              valueData={
                value_ref.digest
                  ? valueDataMap.get(value_ref.digest)
                  : undefined
              }
              json={
                value_ref.digest
                  ? valueDataMap.get(value_ref.digest)?.json?.value
                  : undefined
              }
              renderMode={renderMode}
            />
          ))}
        </>
      )}

      {check.results?.length > 0 && (
        <>
          <Divider />
          <Typography variant="subtitle2">Results</Typography>
          {check.results.map((result, resIdx) => {
            if (!result.data?.length) return null;

            return result.data.map((value_ref, dataIdx) => (
              <AnyDetails
                key={`result-${resIdx}-${dataIdx}`}
                typeUrl={value_ref.typeUrl}
                omitReason={value_ref.omitReason}
                valueData={
                  value_ref.digest
                    ? valueDataMap.get(value_ref.digest)
                    : undefined
                }
                json={
                  value_ref.digest
                    ? valueDataMap.get(value_ref.digest)?.json?.value
                    : undefined
                }
                renderMode={renderMode}
              />
            ));
          })}
        </>
      )}
    </Box>
  );
}
