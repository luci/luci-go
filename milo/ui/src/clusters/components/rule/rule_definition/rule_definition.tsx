// Copyright 2023 The LUCI Authors.
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

import Edit from '@mui/icons-material/Edit';
import Box from '@mui/material/Box';
import IconButton from '@mui/material/IconButton';

import CodeBlock from '@/clusters/components/codeblock/codeblock';
import HelpTooltip from '@/clusters/components/help_tooltip/help_tooltip';

const definitionUnavailableTooltipText =
  'To view this information, your project administrator will need to grant you the "analysis.rules.getDefinition" permission. ' +
  'While you cannot view the rule definition, you can still see matching failures.';

interface Props {
  definition?: string;
  onEditClicked: () => void;
}

const RuleDefinition = ({ definition, onEditClicked }: Props) => {
  if (definition) {
    return (
      <>
        <IconButton
          data-testid="rule-definition-edit"
          onClick={() => onEditClicked()}
          aria-label="edit"
          sx={{ float: 'right' }}
        >
          <Edit />
        </IconButton>
        <Box data-testid="rule-definition" sx={{ display: 'grid' }}>
          <CodeBlock code={definition} />
        </Box>
      </>
    );
  }
  return (
    <>
      <Box sx={{ display: 'inline-block' }} paddingTop={1}>
        <>You do not have permission to view the rule definition.</>
      </Box>
      <HelpTooltip text={definitionUnavailableTooltipText} />
    </>
  );
};

export default RuleDefinition;
