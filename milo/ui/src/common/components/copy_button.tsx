// src/common/components/copy_button.tsx
// Copyright 2025 The LUCI Authors.
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

import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { IconButton, Tooltip, TooltipProps } from '@mui/material';
import { IconButtonProps } from '@mui/material/IconButton';
import React, { useState, useCallback } from 'react';

const DEFAULT_TOOLTIP = 'Copy';
const COPIED_TOOLTIP = 'Copied!';
const ERROR_TOOLTIP = 'Could not copy';
const UNAVAILABLE_TOOLTIP = 'Clipboard API unavailable';

interface CopyButtonProps
  extends Omit<IconButtonProps, 'onClick' | 'title' | 'aria-label'> {
  textToCopy: string;
  tooltipPlacement?: TooltipProps['placement'];
  defaultTooltipText?: string;
  successTooltipText?: string;
  errorTooltipText?: string;
  clipboardUnavailableText?: string;
  icon?: React.ReactNode;
  // Named onCopyAttempt to avoid conflict with standard HTML onCopy event
  onCopyAttempt?: (success: boolean, error?: DOMException | Error) => void;
  ['aria-label']?: string;
}

export function CopyButton({
  textToCopy,
  tooltipPlacement = 'top',
  defaultTooltipText = DEFAULT_TOOLTIP,
  successTooltipText = COPIED_TOOLTIP,
  errorTooltipText = ERROR_TOOLTIP,
  clipboardUnavailableText = UNAVAILABLE_TOOLTIP,
  icon,
  size = 'small',
  onCopyAttempt,
  disabled,
  ...iconButtonProps
}: CopyButtonProps): JSX.Element {
  const [tooltipMessage, setTooltipMessage] = useState(defaultTooltipText);

  const handleCopyToClipboard = useCallback(async () => {
    if (disabled) return;

    if (!navigator.clipboard) {
      setTooltipMessage(clipboardUnavailableText);
      if (onCopyAttempt)
        onCopyAttempt(false, new Error(clipboardUnavailableText));
      setTimeout(() => setTooltipMessage(defaultTooltipText), 3000);
      return;
    }

    try {
      await navigator.clipboard.writeText(textToCopy);
      setTooltipMessage(successTooltipText);
      if (onCopyAttempt) onCopyAttempt(true);
    } catch (err) {
      setTooltipMessage(errorTooltipText);
      if (onCopyAttempt) onCopyAttempt(false, err as DOMException); // Use the new prop name
    } finally {
      // Ensure tooltip resets even if onCopyAttempt throws or is not set
      setTimeout(() => setTooltipMessage(defaultTooltipText), 2000);
    }
  }, [
    textToCopy,
    defaultTooltipText,
    successTooltipText,
    errorTooltipText,
    clipboardUnavailableText,
    onCopyAttempt,
    disabled,
  ]);

  // Determine effective aria-label. If user provides one, use it, else default.
  const effectiveAriaLabel =
    iconButtonProps['aria-label'] || defaultTooltipText;

  return (
    <Tooltip title={tooltipMessage} placement={tooltipPlacement}>
      {/* IconButton might be disabled. Tooltip needs a non-disabled wrapper to work reliably on disabled buttons. */}
      <span>
        <IconButton
          onClick={handleCopyToClipboard}
          size={size}
          disabled={disabled}
          aria-label={effectiveAriaLabel}
          {...iconButtonProps}
        >
          {icon || (
            <ContentCopyIcon
              sx={{ fontSize: iconButtonProps.edge ? undefined : 20 }}
            />
          )}
        </IconButton>
      </span>
    </Tooltip>
  );
}
