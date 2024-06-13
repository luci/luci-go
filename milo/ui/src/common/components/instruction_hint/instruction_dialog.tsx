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

import {
  AlertTitle,
  Box,
  Dialog,
  DialogContent,
  DialogTitle,
  Tab,
  Tabs,
  Typography,
} from '@mui/material';
import Alert from '@mui/material/Alert';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';

import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import {
  Instruction,
  InstructionTarget,
  TargetedInstruction,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/instruction.pb';
import { GetInstructionRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';

const INSTRUCTION_TARGET_DISPLAY_MAP = {
  // The unspecifed target should not happen.
  [InstructionTarget.UNSPECIFIED]: '',
  [InstructionTarget.LOCAL]: 'Local',
  [InstructionTarget.REMOTE]: 'Remote',
  [InstructionTarget.PREBUILT]: 'Prebuilt',
};

export interface InstructionDialogProps {
  readonly open: boolean;
  readonly onClose?: (event: Event) => void;
  readonly container?: HTMLDivElement;
  readonly instructionName: string;
  readonly title: string;
}

export function InstructionDialog({
  open,
  onClose,
  container,
  instructionName,
  title,
}: InstructionDialogProps) {
  // Load instruction.
  const client = useResultDbClient();
  const { isLoading, isError, data } = useQuery({
    ...client.GetInstruction.query(
      GetInstructionRequest.fromPartial({ name: instructionName }),
    ),
    // We only fetch when the dialog is open.
    enabled: open,
  });

  const targetedInstructions = targetedInstructionMap(data);

  const defaultTarget =
    targetedInstructions.size > 0
      ? [...targetedInstructions.keys()][0]
      : InstructionTarget.UNSPECIFIED;

  // State for active target.
  const [currentTarget, setCurrentTarget] = useState(defaultTarget);
  const isCurrentTargetValid = targetedInstructions.has(currentTarget);

  if (!isCurrentTargetValid && targetedInstructions.size > 0) {
    setCurrentTarget(defaultTarget);
  }

  const handleTabChange = (
    e: React.SyntheticEvent<Element, Event>,
    newValue: InstructionTarget,
  ) => {
    e.stopPropagation();
    setCurrentTarget(newValue);
  };

  return (
    <>
      <Dialog
        disablePortal
        open={open}
        onClose={onClose}
        container={container}
        onClick={(e) => e.stopPropagation()}
      >
        <DialogTitle>{title}</DialogTitle>
        <DialogContent>
          {targetedInstructions.size > 0 && (
            <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
              <Tabs value={currentTarget} onChange={handleTabChange}>
                {[...targetedInstructions.keys()].map((target, index) => (
                  <Tab
                    key={index}
                    label={INSTRUCTION_TARGET_DISPLAY_MAP[target]}
                    value={target}
                  />
                ))}
              </Tabs>
            </Box>
          )}
          <Box sx={{ paddingTop: 2 }}>
            {isLoading && <DotSpinner />}
            {isError && (
              <Alert severity="error">
                <AlertTitle>Error</AlertTitle>
                An error occurred while loading the instruction.
              </Alert>
            )}
            {targetedInstructions.size > 0 && (
              <Typography
                component="span"
                sx={{ color: 'var(--default-text-color)' }}
              >
                {targetedInstructions.get(currentTarget)?.content}
              </Typography>
            )}{' '}
          </Box>
        </DialogContent>
      </Dialog>
    </>
  );
}

/**
 * targetedInstructionMap returns a mapping between target and targeted instruction.
 * The map returned will be in ordered: LOCAL, REMOTE, PREBUILT.
 */
function targetedInstructionMap(
  instruction: Instruction | undefined,
): Map<InstructionTarget, TargetedInstruction> {
  const map = new Map<InstructionTarget, TargetedInstruction>();
  if (instruction === undefined) {
    return map;
  }

  // The uniqueness of the target is guaranteed.
  for (const targetedInstruction of instruction.targetedInstructions) {
    for (const target of targetedInstruction.targets) {
      map.set(target, targetedInstruction);
    }
  }
  const sortedMap = new Map(
    [...map.entries()].sort(([target1], [target2]) => target1 - target2),
  );
  return sortedMap;
}
