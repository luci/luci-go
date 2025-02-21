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

import { Close, KeyboardReturn } from '@mui/icons-material';
import { IconButton } from '@mui/material';

import { useInputState, useSetters } from './context';

export function CommitOrClear() {
  const setters = useSetters();
  const inputState = useInputState();

  if (inputState.hasUncommitted) {
    return (
      <IconButton
        title="Press Enter to apply the changes"
        edge="end"
        onClick={() => setters.commit()}
      >
        <KeyboardReturn sx={{ color: 'var(--active-color)' }} />
      </IconButton>
    );
  }

  if (inputState.isEmpty) {
    return <></>;
  }

  return (
    <IconButton title="Clear" edge="end" onClick={() => setters.clear()}>
      <Close sx={{ color: 'var(--delete-color)' }} />
    </IconButton>
  );
}
