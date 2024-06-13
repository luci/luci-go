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
import { act } from 'react';

import {
  Instruction,
  InstructionTarget,
  InstructionType,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/instruction.pb';
import { ResultDBClientImpl } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
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
      .spyOn(ResultDBClientImpl.prototype, 'GetInstruction')
      .mockImplementation(async (_req) => {
        return Instruction.fromPartial({
          id: 'ins_id',
          type: InstructionType.TEST_RESULT_INSTRUCTION,
          targetedInstructions: Object.freeze([
            {
              content: 'test instruction content',
              targets: Object.freeze([
                InstructionTarget.LOCAL,
                InstructionTarget.REMOTE,
              ]),
            },
          ]),
        });
      });

    render(
      <FakeContextProvider>
        <InstructionDialog
          instructionName="invocations/inv/instructions/ins"
          title="Instruction title"
          open={true}
        />
      </FakeContextProvider>,
    );
    // Somehow jest.runAllTimersAsync() times out.
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(screen.getByText('Instruction title')).toBeInTheDocument();
    expect(screen.getByText('Local')).toBeInTheDocument();
    expect(screen.getByText('Remote')).toBeInTheDocument();
    expect(screen.getByText('test instruction content')).toBeInTheDocument();
  });

  it('should show error if failed to load instruction', async () => {
    jest
      .spyOn(ResultDBClientImpl.prototype, 'GetInstruction')
      .mockRejectedValue(
        new GrpcError(RpcCode.PERMISSION_DENIED, 'permission denied'),
      );

    render(
      <FakeContextProvider>
        <InstructionDialog
          instructionName="invocations/inv/instructions/ins"
          title="Instruction title"
          open={true}
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
