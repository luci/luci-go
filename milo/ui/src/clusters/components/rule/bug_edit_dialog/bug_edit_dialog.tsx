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

import LoadingButton from '@mui/lab/LoadingButton';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import LinearProgress from '@mui/material/LinearProgress';
import { Dispatch, SetStateAction, useEffect, useState } from 'react';
import { useParams } from 'react-router';

import BugPicker from '@/clusters/components/bug_picker/bug_picker';
import ErrorAlert from '@/clusters/components/error_alert/error_alert';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import useFetchRule from '@/clusters/hooks/use_fetch_rule';
import { useMutateRule } from '@/clusters/hooks/use_mutate_rule';
import { UpdateRuleRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';

interface Props {
  open: boolean;
  setOpen: Dispatch<SetStateAction<boolean>>;
}

const BugEditDialog = ({ open, setOpen }: Props) => {
  const { project, id: ruleId } = useParams();

  const {
    isPending,
    data: rule,
    error,
  } = useFetchRule(project || '', ruleId || '');

  const [bugId, setBugId] = useState('');

  const mutateRule = useMutateRule(() => {
    setOpen(false);
  });

  useEffect(() => {
    if (rule) {
      // Rules always have a bug set.
      // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
      const bug = rule.bug!;
      setBugId(bug.id);
    }
  }, [rule]);

  if (!ruleId || !project) {
    return (
      <ErrorAlert
        errorText={'Project and/or rule are not defined in the URL'}
        errorTitle="Project and/or rule are undefined"
        showError
      />
    );
  }

  if (error) {
    return <LoadErrorAlert entityName="rule" error={error} />;
  }

  if (isPending || !rule) {
    return <LinearProgress />;
  }

  const handleBugIdChanged = (bugId: string) => {
    setBugId(bugId);
  };

  const handleClose = () => {
    // Rules always have a bug set.
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    const bug = rule.bug!;
    setBugId(bug.id);
    setOpen(false);
  };

  const handleSave = () => {
    const request: UpdateRuleRequest = UpdateRuleRequest.create({
      rule: {
        name: rule.name,
        bug: {
          system: 'buganizer',
          id: bugId,
        },
      },
      updateMask: Object.freeze(['bug']),
      etag: rule.etag,
    });
    mutateRule.mutate(request);
  };

  return (
    <>
      <Dialog open={open} fullWidth>
        <DialogTitle>Change associated bug</DialogTitle>
        <DialogContent sx={{ mt: 1 }}>
          <BugPicker bugId={bugId} handleBugIdChanged={handleBugIdChanged} />
        </DialogContent>
        <DialogActions>
          <Button
            variant="outlined"
            data-testid="bug-edit-dialog-cancel"
            onClick={handleClose}
          >
            Cancel
          </Button>
          <LoadingButton
            variant="contained"
            data-testid="bug-edit-dialog-save"
            onClick={handleSave}
            loading={mutateRule.isPending}
          >
            Save
          </LoadingButton>
        </DialogActions>
      </Dialog>
    </>
  );
};

export default BugEditDialog;
