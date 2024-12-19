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

import { Button, Divider } from '@mui/material';

export type FooterProps = {
  onCancelClick?: (event: object) => void;
  onApplyClick?: (event: object) => void;
};

export function Footer({ onCancelClick, onApplyClick }: FooterProps) {
  return (
    <div>
      <Divider
        component="li"
        sx={{
          paddingTop: '6px',
        }}
      />
      <div
        css={{
          display: 'flex',
          gap: 12,
          padding: '6px 15px 0 30px',
        }}
      >
        <Button disableElevation onClick={onCancelClick}>
          Cancel
        </Button>
        <Button disableElevation variant="contained" onClick={onApplyClick}>
          Apply
        </Button>
      </div>
    </div>
  );
}
