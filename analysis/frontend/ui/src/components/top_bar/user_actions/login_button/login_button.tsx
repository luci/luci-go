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
  useLocation,
} from 'react-router-dom';

import Button from '@mui/material/Button';
import { loginLink } from '@/tools/urlHandling/links';

const LoginButton = () => {
  const location = useLocation();

  return (
    <Button
      data-testid="login_button"
      href={loginLink(location.pathname + location.search + location.hash)}
      key="log in"
      sx={{ color: 'white' }}
    >
      Log in
    </Button>
  );
};

export default LoginButton;
