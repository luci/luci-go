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
  useContext,
  useEffect,
  useState,
} from 'react';
import { useMutation } from 'react-query';
import {
  useNavigate,
  useParams,
  useSearchParams,
} from 'react-router-dom';

import {
  GrpcError,
  RpcCode,
} from '@chopsui/prpc-client';
import LoadingButton from '@mui/lab/LoadingButton';
import Backdrop from '@mui/material/Backdrop';
import CircularProgress from '@mui/material/CircularProgress';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import PanelHeading from '@/components/headings/panel_heading/panel_heading';

import BugPicker from '@/components/bug_picker/bug_picker';
import ErrorAlert from '@/components/error_alert/error_alert';
import RuleEditInput from '@/components/rule_edit_input/rule_edit_input';
import { SnackbarContext } from '@/context/snackbar_context';
import { ClusterId } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import { CreateRuleRequest, Rule } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';
import { linkToRule } from '@/tools/urlHandling/links';
import { getRulesService } from '@/services/services';

const NewRulePage = () => {
  const { project } = useParams();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const [bugSystem, setBugSystem] = useState<string>('');
  const [bugId, setBugId] = useState<string>('');
  const [definition, setDefinition] = useState<string>('');
  const [sourceCluster, setSourceCluster] = useState<ClusterId>({ algorithm: '', id: '' });

  const { setSnack } = useContext(SnackbarContext);
  const createRule = useMutation((request: CreateRuleRequest) => service.create(request));
  const [validationError, setValidationError] = useState<GrpcError | null>(null);

  useEffect(() => {
    const rule = searchParams.get('rule');
    if (rule) {
      setDefinition(rule);
    }
    const sourceClusterAlg = searchParams.get('sourceAlg');
    const sourceClusterID = searchParams.get('sourceId');
    if (sourceClusterAlg && sourceClusterID) {
      setSourceCluster({
        algorithm: sourceClusterAlg,
        id: sourceClusterID,
      });
    }
  }, [searchParams]);


  if (!project) {
    return (
      <ErrorAlert
        errorTitle="Project not found"
        errorText="A project is required to create a rule, please check the URL and try again."
        showError
      />
    );
  }

  const service = getRulesService();

  const handleBugSystemChange = (bugSystem: string) => {
    setBugSystem(bugSystem);
  };

  const handleBugIdChange = (bugId: string) => {
    setBugId(bugId);
  };

  const handleRuleDefinitionChange = (e: ChangeEvent<HTMLTextAreaElement>) => {
    setDefinition(e.target.value);
  };


  const handleSave = () => {
    const request: CreateRuleRequest = {
      parent: `projects/${project}`,
      rule: Rule.create({
        bug: {
          system: bugSystem,
          id: bugId,
        },
        ruleDefinition: definition,
        isActive: true,
        isManagingBug: false,
        isManagingBugPriority: true,
        sourceCluster: sourceCluster,
      }),
    };
    createRule.mutate(request, {
      onSuccess(data) {
        const rule = data;
        setSnack({
          open: true,
          message: 'Rule updated successfully',
          severity: 'success',
        });
        navigate(linkToRule(rule.project, rule.ruleId));
      },
      onError(error) {
        if ( error instanceof GrpcError &&
          error.code === RpcCode.INVALID_ARGUMENT) {
          setValidationError(error);
        } else {
          setSnack({
            open: true,
            message: `Failed to create rule due to: ${error}`,
            severity: 'error',
          });
        }
      },
    });
  };

  return (
    <Container>
      <Paper elevation={3} sx={{
        pt: 1,
        pb: 4,
        px: 2,
        mt: 1,
        mx: 2,
      }}>
        <PanelHeading>
          New Rule
        </PanelHeading>
        <Grid container direction='column' spacing={1}>
          <Grid item xs>
            {validationError && (
              <ErrorAlert
                errorTitle="Validation error"
                errorText={`Rule data is invalid: ${validationError.description.trim()}`}
                showError
                onErrorClose={() => setValidationError(null)}
              />
            )}
          </Grid>
          <Grid item container xs>
            <Grid item xs={6}>
              <Typography>
                Associated bug
              </Typography>
              <BugPicker
                bugSystem={bugSystem}
                bugId={bugId}
                handleBugSystemChanged={handleBugSystemChange}
                handleBugIdChanged={handleBugIdChange} />
            </Grid>
          </Grid>
          <Grid item xs marginTop='1rem'>
            <Typography>
              Rule definition
            </Typography>
            <RuleEditInput
              definition={definition}
              onDefinitionChange={handleRuleDefinitionChange}
            />
          </Grid>
          <Grid item xs>
            <LoadingButton
              variant="contained"
              data-testid="create-rule-save"
              onClick={handleSave}
              loading={createRule.isLoading}>
                Save
            </LoadingButton>
          </Grid>
        </Grid>
      </Paper>
      <Backdrop
        sx={{ color: '#fff' }}
        open={createRule.isSuccess}
      >
        <CircularProgress color="inherit" />
      </Backdrop>
    </Container>
  );
};

export default NewRulePage;
