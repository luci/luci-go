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

import { Description } from '@mui/icons-material';
import { IconButton, styled } from '@mui/material';
import { useState } from 'react';

import { InstructionDialog } from './instruction_dialog';

const StyledIconButton = styled(IconButton)`
  cursor: pointer;
  width: 22px;
  height: 22px;
  border-radius: 2px;
  padding: 2px;
  &:hover {
    background-color: silver;
  }
  display: flex;
  justify-content: center;
  align-items: center;
`;

export interface InstructionHintProps {
  readonly instructionName: string;
  readonly title: string;
  readonly placeholderData: object;
}

export function InstructionHint({
  instructionName,
  title,
  placeholderData,
}: InstructionHintProps) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <StyledIconButton
        sx={{ '& svg': { fontSize: 20 } }}
        onClick={(e) => {
          e.stopPropagation();
          setOpen(true);
        }}
        role="button"
        aria-label="Show reproduction instruction"
        title="Show reproduction instruction"
      >
        <Description />
      </StyledIconButton>

      <InstructionDialog
        open={open}
        onClose={(e) => {
          setOpen(false);
          e.stopPropagation();
        }}
        title={title}
        instructionName={instructionName}
        placeholderData={placeholderData}
      />
    </>
  );
}
