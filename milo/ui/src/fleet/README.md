# Fleet Console UI Subproject

This directory contains the source code for the LUCI Fleet Console UI subproject.

For overall docs on the Fleet Console, see: [go/fleet-console](http://go/fleet-console)

For architectural decisions regarding Material-React-Table usage, see: [mrt-table-architecture.md](./docs/decisions/mrt-table-architecture.md)

## Project Overview

The `fleet` project is a Single Page Application (SPA) that provides a user interface for monitoring and managing the LUCI fleet of bots and drones. It is a subproject of the larger LUCI UI.

- **Live URL:** [go/fleet-console-prod](https://ci.chromium.org/ui/fleet/labs/p/chromeos/devices)

The primary users are:
- **Fleet Operations:** For monitoring and managing the overall health of the fleet.
- **Developers:** For inspecting and debugging the state of devices they are using for tests.

Key functionalities include:
- Viewing and filtering lists of devices.
- Inspecting detailed information for a single device, including its status, properties (dimensions), and current task.
- Performing management actions like running autorepair on devices.

## Architecture

The Fleet project is a client-server application composed of a frontend UI and a dedicated backend service.

- **Frontend:** A Single Page Application built with **TypeScript** and **React**
- **Backend (Fleet Console Server):** A separate service written in **Go**. It manages its own **PostgreSQL** database and is deployed independently on Google Cloud Run. Its codebase resides in `infra/fleetconsole`.
- **Backend Communication:** The frontend communicates with backend services via **gRPC-Web** (using pRPC).
- **Key Backend Services:**
  - **Fleet Console API:** The primary backend for the UI, providing fleet management actions and cached or aggregated data from its own database.
  - **UFS (Unified Fleet System):** An external service that provides comprehensive fleet data, which is cached and served by the Fleet Console API.
  - **Swarming API:** An external service that the UI also interacts with for real-time bot information, events, and task data.

### Additional Code and Configuration Locations
- **API/Data Fetching:** Utilizes `usePrpcServiceClient` and `react-query` hooks for fetching, caching, and managing loading/error states.
- **State Management:** Primarily relies on local component state. React's `createContext` and `useReducer` are avoided for global state management.
- **Testing Philosophy:** Unit tests are used for pure functions and components in isolation. Integration tests are used for testing full user flows (e.g. "click a button and verify a new item appears"). Mock API services and data utilities are used for testing.
- **UI Source:** `infra/go/src/go.chromium.org/luci/milo/ui/src/fleet/`
- **Backend Source:** `infra/go/src/infra/fleetconsole/`
- **Cloud Run Configs:** `infradata/cloud-run configs`
- **Device ACLs:** `data/config/configs/chrome-infra-auth/realms.cfg` (Based on UFS Realms)

## Key Concepts

- **Device:** A physical or virtual machine that is able to handle jobs scheduled against the fleet
- **Bot / Drone:** A Swarming concept for a worker that runs tasks - bots are often associated with devices, but they are a distinct concept.
- **Swarming:** The distributed testing system that manages the fleet of bots and schedules tasks onto them.
- **Dimensions:** Key-value pairs that describe a bot's properties and capabilities (e.g., `os:Linux`, `cpu:x86-64`).
- **Pool:** A label assigned to a group of bots, often used to scope permissions or dedicate capacity.

## Technology Stack

- **Primary Language:** TypeScript
- **Frameworks/Libraries:** React
- **Styling:** CSS-in-JS, React Material UI
- **State Management:** React Hooks
- **Build Tool:** Vite
- **Testing:** Jest, React Testing Library

## Code Structure

The source code is located in the `src/fleet` directory.

```text
src/fleet
├── components/  # Reusable React components (e.g., DeviceTable).
├── config/      # Configuration files for customizable settings.
├── hooks/       # Custom React hooks.
├── pages/       # Top-level components representing a full page/view.
├── layouts/     # Code for our shared Fleet Console layout.
├── utils/       # General shared code helpers.
├── routes.ts    # Defines the routes for the project.
```

## Local Setup and Running

This codebase is hosted as part of Milo UI. To run the frontend, see the overall [Milo dev docs](https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/go.chromium.org/luci/milo/ui/docs/guides/local_development_workflows.md).

### 1. Initial Setup
- Install VSC with `SSH`, `Jest`, and `ESLint` extensions.

### 2. UI Backend / API Server (`/infra/go/src/infra/fleetconsole`)
- Obtain LUCI credentials: `luci-auth login ...`
- Run the server: `go run ./cmd/fleetconsoleserver`
- The API will be available at `http://127.0.0.1:8800/`.

### 3. Web Client (`/infra/go/src/go.chromium.org/luci/milo/ui`)
- Initialize Infra Go environment: `eval ../../../../../env.py`.
- Copy `.env.development` to `.env.development.local` and set `VITE_FLEET_CONSOLE_HOST="localhost:8800"`.
- Run `npm ci` in the `ui` directory.
- Run the dev server: `npm run dev`.
- The UI is available at `http://localhost:8080/ui/fleet/labs/p/chromeos/devices`.

### Set up SSH tunnel to Cloudtop
If working on a Cloudtop, you can set up a tunnel from your local machine via this command:
```sh
ssh -L 8080:localhost:8080 ${cloudtop-name}.c.googlers.com
```
This enables you to access the UI locally at <http://localhost:8080/ui/fleet/labs/p/chromeos/devices>.

## Running Tests

To run TS tests for just the Fleet Console (from the `ui/` root dir):
```sh
npm test -- ./src/fleet/
```

To make it more convenient to code, you can re-run tests automatically based on file changes:
```sh
npm test -- ./src/fleet/ --watch
```

### Generate coverage
To check the test coverage of the codebase:
```sh
npm test -- ./src/fleet --collectCoverageFrom=src/fleet/**/* --coverage
```
The generated coverage report will be available as a website in `ui/coverage/lcov-report/index.html`.

## Release Schedule
- **UI:** Pushed on green, Monday through Thursday (UTC), within ~10 minutes of a CL landing. Pushes are frozen Friday-Sunday.
- **Backend:** Pushed on demand by developers. Please alert the team in the `Lab Management Infra chat` before deploying.

## Documentation Guidelines

### Design Decisions
When documenting new major changes or design decisions in `docs/decisions/`:
1. Focus on the **current architecture** and the **tradeoffs** considered.
2. Avoid logging historical events, dates, or individual names unless crucial for context.
3. Use the standard template: Context, Current Architecture, Design Tradeoffs Considered (Options, Pros/Cons, Decision).
4. Cross-link between frontend and backend docs as needed using relative paths.
5. **Keep migration status current**: Update relevant decision documents to reflect the current technical status quo.

### Critical User Journeys (CUJs)
We maintain records of Critical User Journeys to guide UX design and implementation in `docs/cuj/`.
For guidance on how to write new CUJs, see [README.md](./docs/cuj/README.md).

## Sandbox page
- **Sandbox Page:** `https://ci.chromium.org/ui/fleet/labs/sandbox` for development and experimentation.
