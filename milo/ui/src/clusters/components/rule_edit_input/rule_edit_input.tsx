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

import Link from '@mui/material/Link';
import TextField from '@mui/material/TextField';
import { ChangeEvent } from 'react';

interface Props {
  definition: string;
  onDefinitionChange: (e: ChangeEvent<HTMLTextAreaElement>) => void;
}

const RuleEditInput = ({
  definition,
  onDefinitionChange: handleDefinitionChange,
}: Props) => {
  return (
    <>
      <TextField
        id="rule-definition-input"
        label="Definition"
        multiline
        margin="dense"
        rows={4}
        value={definition}
        onChange={handleDefinitionChange}
        fullWidth
        variant="filled"
        inputProps={{ 'data-testid': 'rule-input' }}
      />
      <small>
        Supported is AND, OR, =,{'<>'}, NOT, IN, LIKE, parentheses and&nbsp;
        <a href="https://cloud.google.com/bigquery/docs/reference/standard-sql/functions-and-operators#regexp_contains">
          REGEXP_CONTAINS
        </a>
        . Valid identifiers are <em>test</em> and <em>reason</em>.{' '}
        <Link href="/ui/tests/help#rules" target="_blank">
          More information
        </Link>
        .
      </small>
    </>
  );
};

export default RuleEditInput;
