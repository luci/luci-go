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

import Typography from '@mui/material/Typography';

interface Props {
  children: React.ReactNode;
}

const PageHeading = ({ children }: Props) => {
  return (
    <Typography
      component="h1"
      variant="h5"
      marginTop="1rem"
      marginBottom="1rem"
    >
      {children}
    </Typography>
  );
};

export default PageHeading;
