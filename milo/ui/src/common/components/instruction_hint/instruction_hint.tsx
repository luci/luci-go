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
import { IconButton, Link, styled } from '@mui/material';
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
  /** render as a link instead of an icon button, using styling to match lit-element links. */
  readonly litLink: boolean;
}

export function InstructionHint({
  instructionName,
  title,
  placeholderData,
  litLink,
}: InstructionHintProps) {
  const [open, setOpen] = useState(false);
  return (
    <>
      {litLink ? (
        <Link
          sx={{
            // These styles are highly specific to the positioning of this element next to links in lit code.
            // They are likely to be incorrect anywhere else, and hence limit the generality of this code.
            color: 'var(--active-text-color)',
            textDecorationColor: 'var(--active-text-color)',
            position: 'relative',
            top: '-1px',
          }}
          component="button"
          onClick={(e) => {
            e.stopPropagation();
            setOpen(true);
          }}
          aria-label="Show reproduction instruction"
          title="Show reproduction instruction"
        >
          rerun instructions
        </Link>
      ) : (
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
      )}

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
