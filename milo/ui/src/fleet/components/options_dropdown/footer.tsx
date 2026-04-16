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

import { Button, Divider, Tooltip } from '@mui/material';

export type FooterProps = {
  footerButtons?: ('cancel' | 'apply' | 'reset')[];
  onCancelClick?: React.MouseEventHandler<HTMLButtonElement>;
  onApplyClick?: React.MouseEventHandler<HTMLButtonElement>;
  onResetClick?: React.MouseEventHandler<HTMLButtonElement>;
  applyDisabled?: boolean;
  applyTooltip?: string;
};

export function Footer({
  footerButtons = ['apply', 'cancel'],
  onResetClick,
  onCancelClick,
  onApplyClick,
  applyDisabled,
  applyTooltip,
}: FooterProps) {
  const applyButton = (
    <Button
      disableElevation
      variant="contained"
      onClick={onApplyClick}
      tabIndex={-1}
      disabled={applyDisabled}
    >
      Apply
    </Button>
  );
  return (
    <div>
      <Divider
        sx={{
          backgroundColor: 'transparent',
          paddingTop: 0,
          marginTop: 0,
          height: 1,
        }}
      />
      <div
        css={{
          display: 'flex',
          gap: 12,
          padding: '6px 30px',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexDirection: 'row-reverse',
        }}
      >
        {footerButtons.includes('apply') &&
          (applyDisabled && applyTooltip ? (
            <Tooltip title={applyTooltip}>
              <span>{applyButton}</span>
            </Tooltip>
          ) : (
            applyButton
          ))}
        {footerButtons.includes('cancel') && (
          <Button disableElevation onClick={onCancelClick} tabIndex={-1}>
            Cancel
          </Button>
        )}
        {footerButtons.includes('reset') && (
          <Button disableElevation onClick={onResetClick} tabIndex={-1}>
            Reset to default
          </Button>
        )}
      </div>
    </div>
  );
}
