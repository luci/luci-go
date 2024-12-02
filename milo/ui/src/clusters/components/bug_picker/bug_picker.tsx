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

import Grid from '@mui/material/Grid2';
import TextField from '@mui/material/TextField';
import { ChangeEvent } from 'react';
import { useParams } from 'react-router-dom';

import ErrorAlert from '@/clusters/components/error_alert/error_alert';

interface Props {
  bugId: string;
  handleBugIdChanged: (bugId: string) => void;
}

const getBugNumber = (bugId: string): string => {
  if (bugId.indexOf('/') >= 0) {
    const parts = bugId.split('/');
    return parts[1];
  } else {
    return bugId;
  }
};

const BugPicker = ({ bugId, handleBugIdChanged }: Props) => {
  const { project } = useParams();

  if (!project) {
    return (
      <ErrorAlert
        showError
        errorTitle="Project not defined"
        errorText={'No project param detected.}'}
      />
    );
  }

  const onBugNumberChange = (e: ChangeEvent<HTMLInputElement>) => {
    const enteredBugId = e.target.value;

    handleBugIdChanged(enteredBugId);
  };

  return (
    <Grid container columnSpacing={1}>
      <Grid size={6}>
        <TextField
          label="Bug number"
          variant="standard"
          inputProps={{ 'data-testid': 'bug-number' }}
          value={getBugNumber(bugId)}
          onChange={onBugNumberChange}
        />
      </Grid>
    </Grid>
  );
};

export default BugPicker;
