// Copyright 2026 The LUCI Authors.
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

import { render, screen, within } from '@testing-library/react';

import { TYPE_URL_WORKNODE_BRIDGE } from '@/chronicle/utils/legacy_worknode';
import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { CheckState } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';
import { WorkPlan } from '@/proto/turboci/graph/orchestrator/v1/workplan.pb';

import { ChronicleContext, ChronicleContextType } from '../context';

import { CheckDetails } from './check_details';

const WP = { id: 'wp' };

function stageWithCheckEdge(
  id: string,
  checkId: string,
  onState: CheckState,
  overrides: Parameters<typeof Stage.fromPartial>[0] = {},
): Stage {
  return Stage.fromPartial({
    identifier: { workPlan: WP, id, isWorknode: false },
    dependencies: {
      edges: [
        {
          check: {
            identifier: { workPlan: WP, id: checkId },
            condition: { onState },
          },
        },
      ],
    },
    ...overrides,
  });
}

function renderCheckDetails(check: Check, graph?: WorkPlan) {
  const ui = (
    <CheckDetails check={check} valueDataMap={new Map<string, ValueData>()} />
  );
  return render(
    graph ? (
      <ChronicleContext.Provider value={{ graph } as ChronicleContextType}>
        {ui}
      </ChronicleContext.Provider>
    ) : (
      ui
    ),
  );
}

describe('CheckDetails', () => {
  const check = Check.fromPartial({
    identifier: { workPlan: WP, id: 'C_target' },
    state: CheckState.CHECK_STATE_WAITING,
    realm: 'android:ci',
  });

  it('renders check metadata without assignment rows when graph is absent', () => {
    renderCheckDetails(check);

    expect(screen.getByText('ID')).toBeInTheDocument();
    expect(screen.getByText('Lwp:CC_target')).toBeInTheDocument();
    expect(screen.getByText('CHECK_STATE_WAITING')).toBeInTheDocument();
    expect(screen.getByText('android:ci')).toBeInTheDocument();
    expect(screen.queryByText('Planned by')).not.toBeInTheDocument();
    expect(screen.queryByText('Executed by')).not.toBeInTheDocument();
  });

  it('lists stages that plan or execute the check and skips unidentifiable stages', () => {
    const graph = WorkPlan.fromPartial({
      identifier: WP,
      checks: [check],
      stages: [
        stageWithCheckEdge(
          'S_plan',
          'C_target',
          CheckState.CHECK_STATE_PLANNING,
        ),
        stageWithCheckEdge(
          'S_exec',
          'C_target',
          CheckState.CHECK_STATE_WAITING,
        ),
        stageWithCheckEdge(
          'bridge_1',
          'C_target',
          CheckState.CHECK_STATE_WAITING,
          {
            identifier: { workPlan: WP, id: 'bridge_1', isWorknode: true },
            args: { typeUrl: TYPE_URL_WORKNODE_BRIDGE },
          },
        ),
        stageWithCheckEdge(
          'S_downstream',
          'C_target',
          CheckState.CHECK_STATE_FINAL,
        ),
        stageWithCheckEdge(
          'S_other',
          'C_other',
          CheckState.CHECK_STATE_WAITING,
        ),
        // Stage lacking identifier must be skipped cleanly.
        Stage.fromPartial({
          dependencies: {
            edges: [
              {
                check: {
                  identifier: { workPlan: WP, id: 'C_target' },
                  condition: { onState: CheckState.CHECK_STATE_WAITING },
                },
              },
            ],
          },
        }),
      ],
    });

    renderCheckDetails(check, graph);

    const plannedByRow = within(screen.getByText('Planned by').parentElement!);
    expect(plannedByRow.getByText('Lwp:SS_plan')).toBeInTheDocument();

    const executedByRow = within(
      screen.getByText('Executed by').parentElement!,
    );
    expect(executedByRow.getByText('Lwp:SS_exec')).toBeInTheDocument();

    expect(screen.queryByText('Lwp:Nbridge_1')).not.toBeInTheDocument();
    expect(screen.queryByText('Lwp:SS_downstream')).not.toBeInTheDocument();
    expect(screen.queryByText('Lwp:SS_other')).not.toBeInTheDocument();
    expect(
      screen.queryByText('<turboci invalid identifier>'),
    ).not.toBeInTheDocument();
  });
});
