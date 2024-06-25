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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { act } from 'react';

import {
  InstructionTarget,
  InstructionType,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/instruction.pb';
import {
  QueryInstructionResponse,
  ResultDBClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { InstructionDialog } from './instruction_dialog';

describe('<InstructionDialog />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it('should render instruction content', async () => {
    jest
      .spyOn(ResultDBClientImpl.prototype, 'QueryInstruction')
      .mockImplementation(async (_req) => {
        return QueryInstructionResponse.fromPartial({
          instruction: {
            id: 'ins_id',
            type: InstructionType.TEST_RESULT_INSTRUCTION,
            targetedInstructions: [
              {
                content: '**test id = {{test.id}}**',
                targets: [InstructionTarget.LOCAL, InstructionTarget.REMOTE],
              },
            ],
          },
          dependencyChains: [
            {
              target: InstructionTarget.LOCAL,
              nodes: [
                {
                  instructionName:
                    'invocations/inv1/instructions/dependency_instruction',
                  content: 'My dependency content',
                },
              ],
            },
            {
              target: InstructionTarget.REMOTE,
              nodes: [],
            },
          ],
        });
      });

    render(
      <FakeContextProvider>
        <InstructionDialog
          instructionName="invocations/inv/instructions/ins"
          title="Instruction title"
          open={true}
          placeholderData={{ test: { id: 'my test' } }}
        />
      </FakeContextProvider>,
    );
    // Somehow jest.runAllTimersAsync() times out.
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(screen.getByText('Instruction title')).toBeInTheDocument();
    expect(screen.getByText('Local')).toBeInTheDocument();
    expect(screen.getByText('Remote')).toBeInTheDocument();
    expect(screen.getByText('test id = my test')).toHaveStyle(
      'font-weight: bold',
    );
    expect(
      screen.getByText('Dependency: dependency_instruction'),
    ).toBeInTheDocument();
    userEvent.click(screen.getByText('Dependency: dependency_instruction'));
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(screen.getByText('My dependency content')).toBeInTheDocument();
  });

  it('should show error if failed to load instruction', async () => {
    jest
      .spyOn(ResultDBClientImpl.prototype, 'QueryInstruction')
      .mockRejectedValue(
        new GrpcError(RpcCode.PERMISSION_DENIED, 'permission denied'),
      );

    render(
      <FakeContextProvider>
        <InstructionDialog
          instructionName="invocations/inv/instructions/ins"
          title="Instruction title"
          open={true}
          placeholderData={{}}
        />
      </FakeContextProvider>,
    );
    // Somehow jest.runAllTimersAsync() times out.
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(
      screen.getByText('An error occurred while loading the instruction.'),
    ).toBeInTheDocument();
  });
});
