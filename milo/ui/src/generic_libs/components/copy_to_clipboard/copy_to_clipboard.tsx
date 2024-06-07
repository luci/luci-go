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

import { ContentCopy, Done } from '@mui/icons-material';
import { Box, styled, SxProps, Theme } from '@mui/material';
import copy from 'copy-to-clipboard';
import { useState } from 'react';
import { useTimeoutFn } from 'react-use';

const Container = styled(Box)`
  display: inline-block;
  cursor: pointer;
  vertical-align: text-bottom;
  width: 18px;
  height: 18px;
  border-radius: 2px;
  padding: 2px;
  font-size: 17px;
  overflow: hidden;

  &:hover {
    background-color: rgba(0, 0, 0, 0.1);
  }
`;

export interface CopyToClipboardProps {
  /**
   * Text, or a function to generated text, to be copied to the clipboard.
   */
  readonly textToCopy: string | (() => string);
  readonly title?: string;
  readonly sx?: SxProps<Theme>;
  readonly className?: string;
}

/**
 * A simple icon that copies textToCopy to clipboard onclick.
 */
export function CopyToClipboard({
  textToCopy,
  title,
  sx,
  className,
}: CopyToClipboardProps) {
  const [justCopied, setJustCopied] = useState(false);
  const [_isReady, _cancel, reset] = useTimeoutFn(() => {
    setJustCopied(false);
  }, 1000);

  function handleCopy() {
    const text = typeof textToCopy === 'function' ? textToCopy() : textToCopy;
    copy(text);
    setJustCopied(true);
    reset();
  }

  return (
    <Container title={title} sx={sx} className={className}>
      {justCopied ? (
        <Done fontSize="inherit" />
      ) : (
        <ContentCopy fontSize="inherit" onClick={handleCopy} />
      )}
    </Container>
  );
}
