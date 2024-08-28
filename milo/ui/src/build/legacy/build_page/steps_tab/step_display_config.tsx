// Copyright 2022 The LUCI Authors.
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

import { MobxLitElement } from '@adobe/lit-mobx';
import createCache from '@emotion/cache';
import { CacheProvider, EmotionCache } from '@emotion/react';
import {
  Box,
  Checkbox,
  FormControl,
  FormControlLabel,
  MenuItem,
  Select,
  SxProps,
  Theme,
  Typography,
} from '@mui/material';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';
import { observer } from 'mobx-react-lite';
import { createRoot, Root } from 'react-dom/client';

import '@/generic_libs/components/dot_spinner';
import {
  consumeStore,
  StoreInstance,
  StoreProvider,
  useStore,
} from '@/common/store';
import { ExpandStepOption } from '@/common/store/user_config/build_config';
import { commonStyles } from '@/common/styles/stylesheets';
import { consumer } from '@/generic_libs/tools/lit_context';

const StepCategoryLabelMap = Object.freeze({
  [ExpandStepOption.All]: 'All',
  [ExpandStepOption.WithNonSuccessful]: 'w/ Non-Successful Child',
  [ExpandStepOption.NonSuccessful]: 'Non-Successful',
  [ExpandStepOption.None]: 'None',
});

const StepCategoryOptionOrder = Object.freeze([
  ExpandStepOption.All,
  ExpandStepOption.WithNonSuccessful,
  ExpandStepOption.NonSuccessful,
  ExpandStepOption.None,
]);

interface LabeledCheckBoxProps {
  readonly label: string;
  readonly checked: boolean;
  readonly onChange: (checked: boolean) => void;
  readonly sx: SxProps<Theme>;
}

const LabeledCheckBox = ({
  label,
  checked,
  onChange,
  sx,
}: LabeledCheckBoxProps) => {
  return (
    <FormControlLabel
      label={label}
      componentsProps={{ typography: { sx: { fontSize: '14px' } } }}
      control={
        <Checkbox
          checked={checked}
          onChange={(e) => onChange(e.target.checked)}
          size="small"
          sx={{ mr: -0.5 }}
        />
      }
      sx={sx}
    ></FormControlLabel>
  );
};

export const StepDisplayConfig = observer(() => {
  const stepsConfig = useStore().userConfig.build.steps;

  return (
    <Box sx={{ mt: -0.5 }}>
      <LabeledCheckBox
        label="Hide Succeeded Steps"
        checked={stepsConfig.elideSucceededSteps}
        onChange={(checked) => stepsConfig.setElideSucceededSteps(checked)}
        sx={{ mr: 1 }}
      />
      <Box
        sx={{
          display: 'inline-grid',
          gridTemplateColumns: 'auto 1fr',
          gap: 1,
          ml: 2,
        }}
      >
        <Typography
          component="span"
          sx={{
            display: 'inline-flex',
            justifyContent: 'center',
            alignContent: 'center',
            flexDirection: 'column',
            fontSize: '14px',
          }}
        >
          Expand Steps:
        </Typography>
        <FormControl sx={{ width: 200 }} size="small">
          <Select
            value={stepsConfig.expandByDefault}
            onChange={(e) =>
              stepsConfig.setExpandByDefault(e.target.value as ExpandStepOption)
            }
            MenuProps={{ disablePortal: true }}
            sx={{
              fontSize: '14px',
              '& .MuiSelect-select': {
                padding: '0.27rem 0.5rem',
              },
            }}
          >
            {StepCategoryOptionOrder.map((category) => (
              <MenuItem
                key={category}
                value={category}
                sx={{ fontSize: '14px' }}
              >
                {StepCategoryLabelMap[category]}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
      </Box>
    </Box>
  );
});

@customElement('milo-bp-step-display-config')
@consumer
export class BuildPageStepDisplayConfigElement extends MobxLitElement {
  @observable.ref @consumeStore() store!: StoreInstance;

  private readonly cache: EmotionCache;
  private readonly parent: HTMLDivElement;
  private readonly root: Root;

  constructor() {
    super();
    makeObservable(this);
    this.parent = document.createElement('div');
    const child = document.createElement('div');
    this.root = createRoot(child);
    this.parent.appendChild(child);
    this.cache = createCache({
      key: 'milo-bp-step-display-config',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <StoreProvider value={this.store}>
          <StepDisplayConfig />
        </StoreProvider>
      </CacheProvider>,
    );
    return this.parent;
  }

  static styles = [commonStyles];
}
