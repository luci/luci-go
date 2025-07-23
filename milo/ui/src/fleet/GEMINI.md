# Gemini Code Assist - LUCI Fleet UI Subproject Context

This document provides context for the LUCI Fleet Console UI subproject to AI code assistants like Gemini.

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

## Key Concepts

Understanding these terms is crucial for working on this project:

- **Device:**  A physical or virtual machine that is able to handle jobs scheduled against the fleet
- **Bot / Drone:** A Swarming concept for a worker that runs tasks - bots are often associated with devices, but they are a distinct concept.
- **Swarming:** The distributed testing system that manages the fleet of bots and schedules tasks onto them. The Fleet UI makes calls to the Swarming API to get bot info, task data, and more.
- **Dimensions:** Key-value pairs that describe a bot's properties and capabilities (e.g., `os:Linux`, `cpu:x86-64`, `pool:luci.chrome.ci`). They are used for filtering bots and for task scheduling.
- **Pool:** A label assigned to a group of bots, often used to scope permissions or dedicate capacity. Dimensions are the primary mechanism for describing bot capabilities; pools are more for grouping.

## Technology Stack

- **Primary Language:** TypeScript
- **Frameworks/Libraries:** React
- **Styling:** CSS-in-JS, React Material UI
- **State Management:** React Hooks
- **Build Tool:** Vite
- **Testing:** Jest, React Testing Library

## Code Structure

The source code is located in the `src/fleet` directory. Where possible, we try to mirror the code and organization conventions used by our parent project, LUCI UI, for code consistency.

```text
src/fleet
├── components/  # Reusable React components (e.g., DeviceTable).
├── config/      # Configuration files that define customizable settings for things like custom column functionality.
├── hooks/       # Custom React hooks.
├── pages/       # Top-level components representing a full page/view (e.g., DeviceListPage, DeviceDetailsPage).
├── layouts/     # Code for our shared Fleet Console layout.
├── utils/       # General shared code helpers.
├── routes.ts    # Defines the routes for the project, as subroutes of `/ui/fleet/`
```

This structure helps organize the code by function, separating UI components from state management and API communication logic.

### Additional Code and Configuration Locations

- **API/Data Fetching:** Utilizes `usePrpcServiceClient` and `react-query` hooks for fetching, caching, and managing loading/error states.
- **State Management:** Primarily relies on local component state. React's `createContext` and `useReducer` are avoided for global state management.
- **Testing Philosophy:** Unit tests are used for pure functions and components in isolation. Integration tests are used for testing full user flows (e.g. "click a button and verify a new item appears"). Mock API services and data utilities are used for testing.
- **UI Source:** `infra/go/src/go.chromium.org/luci/milo/ui/src/fleet/`
- **Backend Source:** `infra/go/src/infra/fleetconsole/`
- **Cloud Run Configs:** `infradata/cloud-run configs`
- **Device ACLs:** `data/config/configs/chrome-infra-auth/realms.cfg` (Based on UFS Realms)

## Release Schedule

- **UI:** Pushed on green, Monday through Thursday (UTC), within ~10 minutes of a CL landing. Pushes are frozen Friday-Sunday.
- **Backend:** Pushed on demand by developers. Please alert the team in the `Lab Management Infra chat` before deploying.

## Local Setup

The recommended local development environment uses Visual Studio Code (VSC) with remote development on a Cloudtop instance.

**1. Initial Setup**

- Install VSC with `SSH`, `Jest`, and `ESLint` extensions.

**2. UI Backend / API Server (`/infra/go/src/infra/fleetconsole`)**

- Obtain LUCI credentials: `luci-auth login ...`
- Run the server: `go run ./cmd/fleetconsoleserver`
- The API will be available at `http://127.0.0.1:8800/`.

**3. Web Client (`/infra/go/src/go.chromium.org/luci/milo/ui`)**

- Initialize Infra Go environment: `eval ../../../../../env.py`.
- Copy `.env.development` to `.env.development.local` and set `VITE_FLEET_CONSOLE_HOST="localhost:8800"`.
- Run `npm ci` in the `ui` directory
- Run the dev server: `npm run dev`. The UI is available at `http://localhost:8080/ui/fleet/labs/p/chromeos/devices`.
- Run tests: `npm test -- ./src/fleet`.

## Sandbox page

- **Sandbox Page:** `https://ci.chromium.org/ui/fleet/labs/sandbox` for development and experimentation.

