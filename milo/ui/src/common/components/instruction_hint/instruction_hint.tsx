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

import { InfoOutlined } from '@mui/icons-material';
import {
  Dialog,
  DialogContent,
  DialogTitle,
  IconButton,
  styled,
} from '@mui/material';
import { useState } from 'react';

const StyledIconButton = styled(IconButton)`
  cursor: pointer;
  width: 24px;
  height: 24px;
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
}

export function InstructionHint({
  instructionName,
  title,
}: InstructionHintProps) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <StyledIconButton
        sx={{ '& svg': { fontSize: 18 } }}
        onClick={(e) => {
          e.stopPropagation();
          setOpen(true);
        }}
        role="button"
        aria-label="Instruction"
        title="Instruction"
      >
        <InfoOutlined />
      </StyledIconButton>

      <Dialog disablePortal open={open} onClose={() => setOpen(false)}>
        <DialogTitle>{title}</DialogTitle>
        <DialogContent>{instructionName}</DialogContent>
      </Dialog>
    </>
  );
}

