---
name: deploy-ui-demo
description: Builds and deploys UI demos/mocks for the ChromeOS Fleet Console/Milo UI to Google App Engine (GAE) for development and demo purposes. Handles GAE version naming constraints, VPC/cache permission workarounds, and ensures workspace cleanup after deployment.
---

# Deploy UI Demo Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill when you need to deploy local UI changes to the App Engine development environment (`luci-milo-dev`) for manual verification or demo purposes.

## Gotchas & Critical Constraints

> [!IMPORTANT]
> **DNS Label Length Limit (63 Characters)**:
> GAE versioned URLs are constructed as:
> `https://<version>-dot-<service>-dot-<project>.appspot.com`
>
> Due to DNS label limits, the concatenated prefix before the first dot (`<version>-dot-<service>-dot-<project>`) **must not exceed 63 characters**.
> - For the `ui-new` service: `-dot-ui-new-dot-luci-milo-dev` takes 29 chars. Maximum version name length is **34 characters**.
> - For the `default` service: `-dot-default-dot-luci-milo-dev` takes 30 chars. Maximum version name length is **33 characters**.
>
> **Rule**: Always use a very short, clean, and unique version name (e.g., `e24d95c-res` or `json-edit-preview`) via the `--target-version=<version>` flag. Do NOT use default generated version names.

> [!IMPORTANT]
> **Unified App Engine Deployment (Required)**:
> Do NOT deploy only the individual `ui-new` service. GAE handles request routing via a central dispatcher service. If you only deploy `ui-new` without deploying the `default` service under the same version ID, requests to static assets (like `/ui/immutable/...`) will fail with 404 console errors, resulting in a blank screen.
> Always upload **both `default` and `ui-new` services together** under the same target version.

> [!WARNING]
> **File Count Limit (10,000 files)**:
> GAE deployments will fail if the number of files uploaded exceeds 10,000. You MUST use `.gcloudignore` files to exclude local `node_modules`, build artifacts, and caches (such as `.cache` and `.tmp`).

> [!IMPORTANT]
> **VPC Connector & Redis Cache Permissions**:
> By default, the production configuration uses a Redis cache and a VPC connector. External developer accounts do not have IAM permissions to access these. You MUST temporarily disable them in `app.yaml` for the mock deployment, and revert those configuration changes immediately after.

> [!TIP]
> **Context Aware Access (CAA) Errors**:
> If the deployment fails with CAA/corporate certificate errors, configure `gcloud` to use client certificates before retrying:
> ```bash
> gcloud config set context_aware/use_client_certificate true
> ```

---

## Workflow

### Step 1: Prepare `.gcloudignore` Files
Create or update `.gcloudignore` files to prevent uploading node modules and caches.

1. Create a `.gcloudignore` file at the root of the `luci` directory:
```
**/node_modules/
**/.cache/
**/.tmp/
```

2. Create a `.gcloudignore` file at `milo/service-ui-new/.gcloudignore`:
```
**/node_modules/
**/.cache/
**/.tmp/
/ui/node_modules/
/ui/.cache/
/ui/.tmp/
```

3. Append the following to `milo/.gcloudignore`:
```
**/node_modules/
**/.cache/
**/.tmp/
```

### Step 2: Temporarily Modify `app.yaml`
Modify `milo/frontend/app.yaml` to disable the Redis cache and comment out the VPC connector.

```diff
-    DS_CACHE: redis
+    DS_CACHE: disable
     VPC_CONNECTOR: projects/luci-milo-dev/locations/us-central1/connectors/connector
     LOGIN_SESSIONS_ROOT_URL: https://luci-milo-dev.appspot.com

-vpc_access_connector:
-  name: ${VPC_CONNECTOR}
+# vpc_access_connector:
+#   name: ${VPC_CONNECTOR}
```

### Step 3: Build the UI Assets
Build the frontend assets with the custom Milo host override pointing to your versioned URL.

Navigate to `milo/ui` and run:

```bash
# Set variables
export MY_VERSION="<short-version-name>" # e.g. "json-edit-preview"
export VITE_OVERRIDE_MILO_HOST="${MY_VERSION}-dot-luci-milo-dev.appspot.com"
export VITE_FLEET_CONSOLE_HOST="fleet-console-dev-1037063051440.us-central1.run.app"
export NODE_OPTIONS="--max-old-space-size=4096"

# Run build
npm run build
```

### Step 4: Deploy to GAE
Deploy the default and ui-new services using `gae.py` via `env.py`.

Navigate to `milo` (one level up from `ui`) and run:

```bash
export GAE_PY_USE_CLOUDBUILDHELPER=1

# Ensure depot_tools is in PATH (typically ~/depot_tools or ~/projects/depot_tools)
export PATH=$PATH:~/depot_tools

# Deploy both default and ui-new services
../../../../env.py gae.py upload -p ./ -A luci-milo-dev default ui-new -f --target-version ${MY_VERSION}
```

*Note: The command may fail at the very end when trying to update datastore indexes or cron jobs due to IAM permissions. This is expected and can be ignored if the services themselves were deployed.*

### Step 5: Verify the Deployment
Verify that the deployed version is serving.

```bash
# Check version status
gcloud app versions list --project luci-milo-dev --filter="id=${MY_VERSION}"

# Verify endpoint responds with HTTP 200
curl -s -o /dev/null -w "%{http_code}" https://${MY_VERSION}-dot-luci-milo-dev.appspot.com/ui/fleet/devices
```

### Step 6: Workspace Cleanup (CRITICAL)

> [!CAUTION]
> **Do NOT run generic commands like `git restore .` or `git checkout .`** as this will erase your actual UI source code modifications. Only revert the temporary deployment configuration files.

Immediately after verifying the deployment, navigate to the `luci` repository root and restore the workspace to its clean state:

```bash
# Revert temporary GAE configuration modifications
git restore milo/frontend/app.yaml milo/.gcloudignore

# Delete temporary untracked gcloudignore files
rm .gcloudignore milo/service-ui-new/.gcloudignore
```

Run `git status` to verify that only your intended UI source code changes remain in the staging area.
