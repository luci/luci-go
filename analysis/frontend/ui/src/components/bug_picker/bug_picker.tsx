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

import { ChangeEvent } from 'react';
import { useParams } from 'react-router-dom';

import CircularProgress from '@mui/material/CircularProgress';
import FormControl from '@mui/material/FormControl';
import Grid from '@mui/material/Grid';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import Select, { SelectChangeEvent } from '@mui/material/Select';
import TextField from '@mui/material/TextField';

import ErrorAlert from '@/components/error_alert/error_alert';
import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import { useFetchProjectConfig } from '@/hooks/use_fetch_project_config';

interface Props {
    bugSystem: string;
    bugId: string;
    handleBugSystemChanged: (bugSystem: string) => void;
    handleBugIdChanged: (bugId: string) => void;
}

const getMonorailSystem = (bugId: string): string | null => {
  if (bugId.indexOf('/') >= 0) {
    const parts = bugId.split('/');
    return parts[0];
  } else {
    return null;
  }
};

const getBugNumber = (bugId: string): string => {
  if (bugId.indexOf('/') >= 0) {
    const parts = bugId.split('/');
    return parts[1];
  } else {
    return bugId;
  }
};

/**
 * An enum representing the supported bug systems.
 *
 * This is needed because mui's <Select> doesn't compare string correctly in typescript.
 */
enum BugSystems {
    EMPTY = '',
    MONORAIL = 'monorail',
    BUGANIZER = 'buganizer',
}

/**
 * This method works around the fact that Select
 * components compare strings for reference.
 *
 * @param {string} bugSystem The bug system to find in the enum.
 * @return {string} A static enum value equal to the string
 *          provided and used in the Select component.
 */
const getStaticBugSystem = (bugSystem: string): string => {
  switch (bugSystem) {
    case '': {
      return BugSystems.EMPTY;
    }
    case 'monorail': {
      return BugSystems.MONORAIL;
    }
    case 'buganizer': {
      return BugSystems.BUGANIZER;
    }
    default: {
      throw new Error('Unknown bug system.');
    }
  }
};

const BugPicker = ({
  bugSystem,
  bugId,
  handleBugSystemChanged,
  handleBugIdChanged,
}: Props) => {
  const { project } = useParams();

  const {
    isLoading,
    data: projectConfig,
    error,
  } = useFetchProjectConfig(project || '');

  if (!project) {
    return (
      <ErrorAlert
        showError
        errorTitle="Project not defined"
        errorText={'No project param detected.}'}/>
    );
  }

  const selectedBugSystem = getStaticBugSystem(bugSystem);

  if (isLoading || !projectConfig) {
    return (
      <Grid container justifyContent="center">
        <CircularProgress data-testid="circle-loading" />
      </Grid>
    );
  }

  if (error) {
    return <LoadErrorAlert
      entityName="project config"
      error={error}
    />;
  }

  const monorailSystem = getMonorailSystem(bugId);

  const onBugSystemChange = (e: SelectChangeEvent<typeof bugSystem>) => {
    handleBugSystemChanged(e.target.value);

    // When the bug system changes, we also need to update the Bug ID.
    if (e.target.value == 'monorail') {
      // Switching to monorail is disabled if the project config does not have
      // monorail details, so we can assume it exists.
      // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
      const monorailConfig = projectConfig.bugManagement!.monorail!;
      handleBugIdChanged(`${monorailConfig.project}/${getBugNumber(bugId)}`);
    } else if (e.target.value == 'buganizer') {
      handleBugIdChanged(getBugNumber(bugId));
    }
  };

  const onBugNumberChange = (e: ChangeEvent<HTMLInputElement>) => {
    const enteredBugId = e.target.value;

    if (monorailSystem != null) {
      handleBugIdChanged(`${monorailSystem}/${enteredBugId}`);
    } else {
      handleBugIdChanged(enteredBugId);
    }
  };

  return (
    <Grid container item columnSpacing={1}>
      <Grid item xs={6}>
        <FormControl variant="standard" fullWidth>
          <InputLabel id="bug-picker_select-bug-tracker-label">Bug tracker</InputLabel>
          <Select
            labelId="bug-picker_select-bug-tracker-label"
            id="bug-picker_select-bug-tracker"
            value={selectedBugSystem}
            onChange={onBugSystemChange}
            variant="standard"
            inputProps={{ 'data-testid': 'bug-system' }}>
            <MenuItem value={getStaticBugSystem('monorail')} disabled={!projectConfig.bugManagement?.monorail}>
              {projectConfig.bugManagement?.monorail?.displayPrefix || 'monorail'}
            </MenuItem>
            <MenuItem value={getStaticBugSystem('buganizer')}>
                Buganizer
            </MenuItem>
          </Select>
        </FormControl>
      </Grid>
      <Grid item xs={6}>
        <TextField
          label="Bug number"
          variant="standard"
          inputProps={{ 'data-testid': 'bug-number' }}
          value={getBugNumber(bugId)}
          onChange={onBugNumberChange}/>
      </Grid>
    </Grid>
  );
};

export default BugPicker;
