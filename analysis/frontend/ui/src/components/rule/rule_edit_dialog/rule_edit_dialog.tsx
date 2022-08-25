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

import {
  ChangeEvent,
  Dispatch,
  SetStateAction,
  useState,
} from 'react';

import LoadingButton from '@mui/lab/LoadingButton';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import TextField from '@mui/material/TextField';

import { useMutateRule } from '../../../hooks/useMutateRule';
import {
  Rule,
  UpdateRuleRequest,
} from '../../../services/rules';

interface Props {
    open: boolean;
    setOpen: Dispatch<SetStateAction<boolean>>;
    rule: Rule;
}


const RuleEditDialog = ({
  open = false,
  setOpen,
  rule,
}: Props) => {
  const [currentRuleDefinition, setCurrentRuleDefinition] = useState(rule.ruleDefinition);

  const mutateRule = useMutateRule(() => {
    setOpen(false);
  });
  const handleDefinitionChange = (e: ChangeEvent<HTMLTextAreaElement>) => {
    setCurrentRuleDefinition(e.target.value);
  };

  const handleClose = () => {
    setCurrentRuleDefinition(() => rule.ruleDefinition);
    setOpen(() => false);
  };

  const handleSave = () => {
    const request: UpdateRuleRequest = {
      rule: {
        name: rule.name,
        ruleDefinition: currentRuleDefinition,
      },
      updateMask: 'ruleDefinition',
      etag: rule.etag,
    };
    mutateRule.mutate(request);
  };

  return (
    <Dialog
      open={open}
      maxWidth="lg"
      fullWidth>
      <DialogTitle>Edit rule definition</DialogTitle>
      <DialogContent>
        <TextField
          id="rule-definition-input"
          label="Definition"
          multiline
          margin="dense"
          rows={4}
          value={currentRuleDefinition}
          onChange={handleDefinitionChange}
          fullWidth
          variant="filled"
          inputProps={{ 'data-testid': 'rule-input' }}/>
        <small>
            Supported is AND, OR, =,{'<>'}, NOT, IN, LIKE, parentheses and <a href="https://cloud.google.com/bigquery/docs/reference/standard-sql/functions-and-operators#regexp_contains">REGEXP_CONTAINS</a>.
            Valid identifiers are <em>test</em> and <em>reason</em>.
        </small>
      </DialogContent>
      <DialogActions>
        <Button
          variant="outlined"
          data-testid="rule-edit-dialog-cancel"
          onClick={handleClose}>
            Cancel
        </Button>
        <LoadingButton
          variant="contained"
          data-testid="rule-edit-dialog-save"
          onClick={handleSave}
          loading={mutateRule.isLoading}>
            Save
        </LoadingButton>
      </DialogActions>
    </Dialog>
  );
};

export default RuleEditDialog;
