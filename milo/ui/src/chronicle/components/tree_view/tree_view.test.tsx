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

import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { CheckState } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageConcludedReason } from '@/proto/turboci/graph/orchestrator/v1/stage_concluded_reason.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';
import { WorkPlan } from '@/proto/turboci/graph/orchestrator/v1/workplan.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { WorkflowType } from '../../fake_turboci_graph';
import { TYPE_URL_BUILD_RESULT } from '../../utils/check_utils';
import { ChronicleContext, ChronicleContextType } from '../context';

import { Component as TreeView } from './tree_view';

jest.mock('@/generic_libs/components/routed_tabs/context', () => ({
  ...jest.requireActual('@/generic_libs/components/routed_tabs/context'),
  useDeclareTabId: jest.fn(),
}));

const mockContext: ChronicleContextType = {
  workplanId: 'demo',
  graph: undefined,
  valueDataMap: new Map(),
  activeEnvironment: 'demo',
  setActiveEnvironment: jest.fn(),
  workflowType: WorkflowType.ANDROID,
  setWorkflowType: jest.fn(),
  selectedNodeId: undefined,
  setSelectedNodeId: jest.fn(),
  detecting: false,
  setDetecting: jest.fn(),
  detectionFailed: false,
  setDetectionFailed: jest.fn(),
  showEnvDialog: false,
  setShowEnvDialog: jest.fn(),
  foundEnvironments: [],
  requestedEnvFailed: undefined,
  failedEnvironments: [],
  detectionCancelled: false,
  setDetectionCancelled: jest.fn(),
};

function createLegacyWorkNodeValueData(
  type: string,
  success: boolean,
): ValueData {
  return ValueData.fromPartial({
    json: {
      value: JSON.stringify({
        workExecutorType: 'PENDING_CHANGE_BUILD',
        workParameters: {
          releaseRequest: { type },
        },
        workOutput: { success },
      }),
    },
  });
}

function createBuildResultValueData(success: boolean): ValueData {
  return ValueData.fromPartial({
    json: {
      value: JSON.stringify({ success }),
    },
  });
}

function createNStage(
  id: string,
  state: number,
  digest?: string,
  concludedReason?: StageConcludedReason,
): Stage {
  return Stage.fromPartial({
    identifier: { id, isWorknode: true },
    state,
    concludedReason,
    ...(digest ? { legacy: { worknode: { digest } } } : {}),
  });
}

function createCheck(id: string, state: CheckState, digest?: string): Check {
  return Check.fromPartial({
    identifier: { id },
    state,
    ...(digest
      ? {
          results: [
            {
              data: [{ digest, typeUrl: TYPE_URL_BUILD_RESULT }],
            },
          ],
        }
      : {}),
  });
}

function renderTreeView(
  graph: WorkPlan,
  valueDataMap: Map<string, ValueData> = new Map(),
) {
  return render(
    <FakeContextProvider>
      <ChronicleContext.Provider
        value={{
          ...mockContext,
          graph,
          valueDataMap,
        }}
      >
        <TreeView />
      </ChronicleContext.Provider>
    </FakeContextProvider>,
  );
}

describe('TreeView', () => {
  it('renders status icons for N-stages correctly', () => {
    const valueDataMap = new Map<string, ValueData>([
      ['digest-success', createLegacyWorkNodeValueData('success_node', true)],
      ['digest-fail', createLegacyWorkNodeValueData('fail_node', false)],
    ]);

    const graph = WorkPlan.fromPartial({
      stages: [
        createNStage('stage-n-success', 40, 'digest-success'), // FINAL
        createNStage('stage-n-fail', 40, 'digest-fail'), // FINAL
        createNStage(
          'stage-n-canceled',
          40,
          undefined,
          StageConcludedReason.STAGE_CONCLUDED_REASON_CANCELLED,
        ), // FINAL canceled
        createNStage('stage-n-running', 20), // ATTEMPTING
        createNStage('stage-n-planned', 10), // PLANNED
      ],
      checks: [],
    });

    renderTreeView(graph, valueDataMap);

    const tree = screen.getByRole('tree');
    // Expect node labels inside the tree view
    expect(within(tree).getByText('build successNode')).toBeInTheDocument();
    expect(within(tree).getByText('build failNode')).toBeInTheDocument();
    expect(
      within(tree).getByText('Stage: stage-n-canceled'),
    ).toBeInTheDocument();
    expect(
      within(tree).getByText('Stage: stage-n-running'),
    ).toBeInTheDocument();
    expect(
      within(tree).getByText('Stage: stage-n-planned'),
    ).toBeInTheDocument();

    // Verify icons are rendered for statuses
    expect(within(tree).getByTestId('CheckCircleIcon')).toBeInTheDocument();
    expect(within(tree).getByTestId('ErrorIcon')).toBeInTheDocument();
    expect(within(tree).getByTestId('CancelIcon')).toBeInTheDocument();
    expect(within(tree).getByTestId('AutorenewIcon')).toBeInTheDocument();
    expect(
      within(tree).getAllByTestId('FiberManualRecordIcon').length,
    ).toBeGreaterThanOrEqual(1);
  });

  it('renders status icons for Checks correctly', () => {
    const valueDataMap = new Map<string, ValueData>([
      ['digest-chk-success', createBuildResultValueData(true)],
      ['digest-chk-fail', createBuildResultValueData(false)],
    ]);

    const graph = WorkPlan.fromPartial({
      stages: [],
      checks: [
        createCheck(
          'check-success',
          CheckState.CHECK_STATE_FINAL,
          'digest-chk-success',
        ),
        createCheck(
          'check-fail',
          CheckState.CHECK_STATE_FINAL,
          'digest-chk-fail',
        ),
        createCheck('check-pending', CheckState.CHECK_STATE_PLANNING),
      ],
    });

    renderTreeView(graph, valueDataMap);

    const tree = screen.getByRole('tree');
    expect(within(tree).getByText('Check: check-success')).toBeInTheDocument();
    expect(within(tree).getByText('Check: check-fail')).toBeInTheDocument();
    expect(within(tree).getByText('Check: check-pending')).toBeInTheDocument();

    expect(within(tree).getByTestId('CheckCircleIcon')).toBeInTheDocument();
    expect(within(tree).getByTestId('ErrorIcon')).toBeInTheDocument();
    expect(
      within(tree).getAllByTestId('FiberManualRecordIcon').length,
    ).toBeGreaterThanOrEqual(1);
  });
});
