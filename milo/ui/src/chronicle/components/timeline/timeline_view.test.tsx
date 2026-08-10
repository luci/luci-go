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

import { fireEvent, render, screen } from '@testing-library/react';
import React from 'react';
import { VirtuosoMockContext } from 'react-virtuoso';

import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageState } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';
import { WorkPlan } from '@/proto/turboci/graph/orchestrator/v1/workplan.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { WorkflowType } from '../../fake_turboci_graph';
import { ChronicleContext, ChronicleContextType } from '../context';

import { Component as TimelineView } from './timeline_view';

jest.mock('@/generic_libs/components/routed_tabs/context', () => ({
  ...jest.requireActual('@/generic_libs/components/routed_tabs/context'),
  useDeclareTabId: jest.fn(),
}));

// Mock react-resizable-panels to simple div wrappers during unit tests.
// This avoids JSDOM-specific event listener limitations (e.g. AbortSignal errors)
// while allowing tests to focus strictly on TimelineView rendering and selection state.
jest.mock('react-resizable-panels', () => ({
  PanelGroup: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  Panel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  PanelResizeHandle: () => <div>ResizeHandle</div>,
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

function createTestStage(): Stage {
  return Stage.create({
    identifier: { id: 'stage-uuid-1234', isWorknode: true },
    legacy: {
      worknode: {
        digest: 'digest-1',
      },
    },
    stateHistory: [
      {
        state: StageState.STAGE_STATE_ATTEMPTING,
        version: { ts: '2026-07-23T10:00:00Z' },
      },
      {
        state: StageState.STAGE_STATE_FINAL,
        version: { ts: '2026-07-23T10:05:00Z' },
      },
    ],
  });
}

function createTestValueDataMap(): Map<string, ValueData> {
  return new Map<string, ValueData>([
    [
      'digest-1',
      ValueData.create({
        json: {
          value: JSON.stringify({
            workExecutorType: 'PENDING_CHANGE_BUILD',
            workParameters: {
              releaseRequest: { type: 'FASTBUILD' },
            },
          }),
        },
      }),
    ],
  ]);
}

describe('TimelineView', () => {
  it('renders timeline with stage label from getStageLabel helper', () => {
    const stage = createTestStage();
    const valueDataMap = createTestValueDataMap();
    const graph: WorkPlan = WorkPlan.create({
      identifier: { id: 'demo' },
      stages: [stage],
      checks: [],
    });

    render(
      <FakeContextProvider>
        <VirtuosoMockContext.Provider
          value={{ viewportHeight: 800, itemHeight: 30 }}
        >
          <ChronicleContext.Provider
            value={{ ...mockContext, graph, valueDataMap }}
          >
            <TimelineView />
          </ChronicleContext.Provider>
        </VirtuosoMockContext.Provider>
      </FakeContextProvider>,
    );

    // Human-readable stage label extracted via getStageLabel should be rendered
    expect(screen.getByText('build fastbuild')).toBeInTheDocument();
  });

  it('triggers setSelectedNodeId when clicking a stage item', () => {
    const stage = createTestStage();
    const valueDataMap = createTestValueDataMap();
    const graph: WorkPlan = WorkPlan.create({
      identifier: { id: 'demo' },
      stages: [stage],
      checks: [],
    });
    const setSelectedNodeId = jest.fn();

    render(
      <FakeContextProvider>
        <VirtuosoMockContext.Provider
          value={{ viewportHeight: 800, itemHeight: 30 }}
        >
          <ChronicleContext.Provider
            value={{
              ...mockContext,
              graph,
              valueDataMap,
              setSelectedNodeId,
            }}
          >
            <TimelineView />
          </ChronicleContext.Provider>
        </VirtuosoMockContext.Provider>
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByText('build fastbuild'));
    expect(setSelectedNodeId).toHaveBeenCalledWith('stage-uuid-1234');
  });

  it('renders InspectorPanel when a stage is selected', () => {
    const stage = createTestStage();
    const valueDataMap = createTestValueDataMap();
    const graph: WorkPlan = WorkPlan.create({
      identifier: { id: 'demo' },
      stages: [stage],
      checks: [],
    });

    render(
      <FakeContextProvider>
        <VirtuosoMockContext.Provider
          value={{ viewportHeight: 800, itemHeight: 30 }}
        >
          <ChronicleContext.Provider
            value={{
              ...mockContext,
              graph,
              valueDataMap,
              selectedNodeId: 'stage-uuid-1234',
            }}
          >
            <TimelineView />
          </ChronicleContext.Provider>
        </VirtuosoMockContext.Provider>
      </FakeContextProvider>,
    );

    // Inspector panel renders "Stage Details" header when selected
    expect(screen.getByText('Stage Details')).toBeInTheDocument();
  });
});
