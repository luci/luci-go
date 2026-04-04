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

import { Alert, AlertTitle, Box } from '@mui/material';

// TODO: Remove, add redirection after 2-3 weeks.
export const BotData = () => {
  return (
    <Box>
      <Alert severity="warning" sx={{ mb: 2 }}>
        <AlertTitle>Bot information has been moved!</AlertTitle>
        <div>Go to Dimensions tab to find information about the bot.</div>
        <div>Bot dimensions has been merged with device dimensions.</div>
      </Alert>
    </Box>
  );
};
