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

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import {
  renderMustacheMarkdown,
  targetedInstructionMap,
} from '@/common/tools/instruction/instruction_utils';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import { InstructionTarget } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/instruction.pb';
import { QueryInstructionRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { InstructionDependency } from './instruction_dependency';
import { codeBlockStyles } from './style';

const INSTRUCTION_TARGET_DISPLAY_MAP = {
  // The unspecifed target should not happen.
  [InstructionTarget.INSTRUCTION_TARGET_UNSPECIFIED]: '',
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
  // placeholder data is to be passed to Mustache.render to replace the placeholders.
  readonly placeholderData: object;
}

export function InstructionDialog({
  open,
  onClose,
  container,
  instructionName,
  title,
  placeholderData,
}: InstructionDialogProps) {
  // Load instruction.
  const client = useResultDbClient();
  const { isPending, isError, data } = useQuery({
    ...client.QueryInstruction.query(
      QueryInstructionRequest.fromPartial({ name: instructionName }),
    ),
    // We only fetch when the dialog is open.
    enabled: open,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
  });

  const targetedInstructions = targetedInstructionMap(data?.instruction);

  const defaultTarget =
    targetedInstructions.size > 0
      ? [...targetedInstructions.keys()][0]
      : InstructionTarget.INSTRUCTION_TARGET_UNSPECIFIED;

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

  const targetedInstruction = targetedInstructions.get(currentTarget);
  const chain = data?.dependencyChains.find(
    (chain) => chain.target === currentTarget,
  );

  return (
    <>
      <Dialog
        disablePortal
        open={open}
        onClose={onClose}
        container={container}
        onClick={(e) => e.stopPropagation()}
        fullWidth
        maxWidth="xl"
      >
        <DialogTitle>{title}</DialogTitle>
        <DialogContent>
          {targetedInstructions.size > 0 && (
            <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
              <Tabs value={currentTarget} onChange={handleTabChange}>
                {[...targetedInstructions.keys()].map((target, index) => (
                  <Tab
                    sx={{ color: 'black' }}
                    key={index}
                    label={INSTRUCTION_TARGET_DISPLAY_MAP[target]}
                    value={target}
                  />
                ))}
              </Tabs>
            </Box>
          )}
          <Box
            sx={{
              paddingTop: 2,
              overflowWrap: 'break-word',
              wordBreak: 'break-all',
              whiteSpace: 'pre-wrap',
            }}
          >
            {isPending && <DotSpinner />}
            {isError && (
              <Alert severity="error">
                <AlertTitle>Error</AlertTitle>
                An error occurred while loading the instruction.
              </Alert>
            )}
            {chain && (
              <>
                {chain.nodes
                  .slice()
                  .reverse()
                  .map((node, index) => (
                    <InstructionDependency key={index} dependencyNode={node} />
                  ))}
              </>
            )}
            {targetedInstructions.size > 0 && (
              <Typography component="span" css={codeBlockStyles}>
                <SanitizedHtml
                  html={renderMustacheMarkdown(
                    targetedInstruction?.content,
                    placeholderData,
                  )}
                />
              </Typography>
            )}
          </Box>
        </DialogContent>
      </Dialog>
    </>
  );
}
