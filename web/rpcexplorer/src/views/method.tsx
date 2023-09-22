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

import { useEffect, useMemo, useRef, useState } from 'react';
import { useParams, useSearchParams } from 'react-router-dom';

import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Grid from '@mui/material/Grid';
import LinearProgress from '@mui/material/LinearProgress';

import { useGlobals } from '../context/globals';

import { AuthMethod, AuthSelector } from '../components/auth_selector';
import { Doc } from '../components/doc';
import { ErrorAlert } from '../components/error_alert';
import { ExecuteIcon } from '../components/icons';
import { OAuthError } from '../data/oauth';

import { RequestEditor, RequestEditorRef } from '../components/request_editor';
import { ResponseEditor } from '../components/response_editor';


const Method = () => {
  const { serviceName, methodName } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const { descriptors, oauthClient } = useGlobals();
  const [authMethod, setAuthMethod] = useState(AuthMethod.load());
  const [running, setRunning] = useState(false);
  const [response, setResponse] = useState('');
  const [error, setError] = useState<Error | null>(null);

  // Request editor is used via imperative methods since it can be too sluggish
  // to update on key presses otherwise.
  const requestEditor = useRef<RequestEditorRef>(null);

  // Initial request body can be passed via `request` query parameter.
  // Pretty-print it if it is a valid JSON. Memo this to avoid reparsing
  // potentially large JSON all the time.
  let initialRequest = searchParams.get('request') || '{}';
  initialRequest = useMemo(() => {
    try {
      return JSON.stringify(JSON.parse(initialRequest), null, 2);
    } catch {
      return initialRequest;
    }
  }, [initialRequest]);

  // Persist changes to `authMethod` in the local storage.
  useEffect(() => AuthMethod.store(authMethod), [authMethod]);

  // Find the method descriptor. It will be used for auto-completion and for
  // actually invoking the method.
  const svc = descriptors.service(serviceName ?? 'unknown');
  if (svc === undefined) {
    return (
      <Alert severity='error'>
        Service <b>{serviceName ?? 'unknown'}</b> is not
        registered in the server.
      </Alert>
    );
  }
  const method = svc.method(methodName ?? 'unknown');
  if (method === undefined) {
    return (
      <Alert severity='error'>
        Method <b>{methodName ?? 'unknown'}</b> is not a part of
        <b>{serviceName ?? 'unknown'}</b> service.
      </Alert>
    );
  }

  const invokeMethod = () => {
    if (!requestEditor.current) {
      return;
    }

    // Try to get a parsed JSON request from the editor. Catch bad JSON errors.
    let parsedReq: object;
    try {
      parsedReq = requestEditor.current.prepareRequest();
    } catch (err) {
      if (err instanceof Error) {
        setError(err);
      } else {
        setError(new Error(`${err}`));
      }
      return;
    }

    // Use compact request serialization (strip spaces etc).
    const normalizedReq = JSON.stringify(parsedReq);

    // Update the current location to allow copy-pasting this request via URI.
    setSearchParams((params) => {
      if (normalizedReq != '{}') {
        params.set('request', normalizedReq);
      } else {
        params.delete('request');
      }
      return params;
    }, { replace: true });

    // Deactivate the UI while the request is running.
    setRunning(true);

    // Grabs the authentication header and invokes the method.
    const authAndInvoke = async () => {
      let authorization = '';
      if (authMethod == AuthMethod.OAuth) {
        authorization = `Bearer ${await oauthClient.accessToken()}`;
      }
      return await method.invoke(normalizedReq, authorization);
    };
    authAndInvoke()
        .then((response) => {
          setResponse(response);
          setError(null);
        })
        .catch((error) => {
          // Canceled OAuth flow is a user-initiated error, don't show it.
          if (!(error instanceof OAuthError && error.cancelled)) {
            setResponse('');
            setError(error);
          }
        })
        .finally(() => {
          // Reactive the UI.
          setRunning(false);
        });
  };

  return (
    <Grid container spacing={2}>
      <Grid item xs={12}>
        <Doc markdown={method.doc} />
      </Grid>

      <Grid item xs={12}>
        <RequestEditor
          ref={requestEditor}
          requestType={descriptors.message(method.requestType)}
          defaultValue={initialRequest}
          readOnly={running}
          onInvokeMethod={invokeMethod}
        />
      </Grid>

      <Grid item xs={2}>
        <Button
          variant='outlined'
          disabled={running}
          onClick={invokeMethod}
          endIcon={<ExecuteIcon />}>
          Execute
        </Button>
      </Grid>

      <Grid item xs={10}>
        <Box>
          <AuthSelector
            selected={authMethod}
            onChange={setAuthMethod}
            oauthClientId={oauthClient.clientId}
            disabled={running}
          />
        </Box>
      </Grid>

      {error &&
        <Grid item xs={12}>
          <ErrorAlert error={error} />
        </Grid>
      }

      {running &&
        <Grid item xs={12}>
          <LinearProgress />
        </Grid>
      }

      <Grid item xs={12}>
        <ResponseEditor value={response} />
      </Grid>
    </Grid>
  );
};


export default Method;
