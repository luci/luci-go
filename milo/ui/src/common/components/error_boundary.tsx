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

import styled from '@emotion/styled';
import { Alert, AlertTitle, Link } from '@mui/material';
import { Component, ReactNode } from 'react';

import { genFeedbackUrl } from '@/common/tools/utils';

export interface ErrorBoundaryProps {
  readonly children: ReactNode;
}

interface ErrorBoundaryState {
  error?: Error;
}

const ErrorDisplay = styled.pre({
  whiteSpace: 'pre-wrap',
  overflowWrap: 'break-word',
});

export class ErrorBoundary extends Component<
  ErrorBoundaryProps,
  ErrorBoundaryState
> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = {};
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  render() {
    if (this.state.error) {
      return (
        <Alert severity="error">
          <AlertTitle>Error</AlertTitle>
          <ErrorDisplay>{this.state.error.message}</ErrorDisplay>
          <Link
            href={genFeedbackUrl(this.state.error.message)}
            target="_blank"
            rel="noopener"
          >
            File a bug
          </Link>
        </Alert>
      );
    }

    return this.props.children;
  }
}
