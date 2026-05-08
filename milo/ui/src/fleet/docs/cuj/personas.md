# Fleet Console User Personas

This directory defines the primary human actors who interact with the Fleet Console. Grounding our feature implementations, layout choices, and automated code-generation in these concrete profiles ensures our high-density views directly optimize real-world human processing speeds.

---

## Terminology Note: "Fleet Steward"
In legacy documentation, the term **Fleet Steward** was used as a broad umbrella concept to encompass both hardware-facing and software-facing fleet maintainers. While we still occasionally use this term to refer to anyone managing general device health, our engineering team distinguishes strictly between **Lab Technicians** (physical operations) and **Fleet Engineers** (infrastructure, toolchain, and automation development).

---

## The Persona Map

The following matrix maps our distinct user roles to their primary focus areas, system constraints, and success vectors.

| Persona | Primary Focus Area | Core System Tooling / Interfaces | Primary Success Metric |
| :--- | :--- | :--- | :--- |
| **Lab Technician** | Repair Operations & Physical Capacity | Physical lab floor, Fleet Management CLI, repair tools, local terminal scripts | Minimize physical walking & device diagnostic time |
| **Fleet Engineer** | Fleet Health Management | Code repositories, Swarming, Mobile Harness, Milo build logs | Scaling fleet automation, reducing manual triage toil |
| **Resource Requester / Owner** | Resource Delivery | Issue Tracker, request dashboards, target testing frameworks | Minimize capacity wait times & maximize owned node utilization |
| **Resource Planner** | Resource Delivery | Forecasting models, budget planners, Asset Management System | Zero-toil request verification and demand alignment |
| **Fulfillment Manager** | Resource Delivery | Provisioning trackers, Issue Tracker, rack-build tickets | Minimizing hardware delivery lead times & unblocking builds |
| **Fleet Developer / Oncall** | Fleet Health Management | Swarming, Milo build interfaces, local developer CLIs | Fast remote device leasing & direct log debugging |

---

## Individual Persona Profiles

### 1. The Lab Technician ("The Hands-On Fixer")
> *"I need to know exactly which machines to touch, in what physical order, so I can fix them and get out of the hot aisle."*

*   **Role Context:** Spends up to 90% of their shift physically on the lab floor, handling hardware swaps, cabling, and basic visual diagnostics.
*   **Operational Focus:** Physical interventions. Needs simple, high-density coordinate sorting to batch physical runs.
*   **Atypical Skills:** High hardware expertise; very low patience for complex, multi-step command-line interfaces or desktop-oriented form screens.
*   **Operational Pain Points:**
    *   Wasting time walking back and forth across massive server rows due to lack of contiguous spatial grouping in task lists.
    *   Siloed tracking forcing them to copy notes into the Issue Tracker tickets manually while checking separate inventory databases.

### 2. The Fleet Engineer ("The Automation Builder")
> *"My goal is keeping the test pools saturated and healthy through systemic fixes. I care about aggregate reliability, trends, and writing code to prevent recurring failures."*

*   **Role Context:** Sits at a desk managing large-scale, cross-platform host environments.
*   **Operational Focus:** Codebase, automated recovery services, and infrastructure scaling.
*   **Atypical Skills:** Expert knowledge of our logical test architectures, Swarming pools, and automated scheduling systems.
*   **Operational Pain Points:**
    *   Investigating failures requires clicking through 4 to 5 tools (Swarming -> Milo -> Lab Management System) to verify a single error signature.
    *   Inaccurate metrics dashboards displaying low-utilization battery or testing nodes as "dead" without team-specific context.

### 3. The Resource Requester & Owner ("The Customer")
> *"I'm trying to run critical test suites for an upcoming release. If I don't have enough hardware online, our release slips."*

*   **Role Context:** A software engineer or working group leader focused on compiling and running test suites.
*   **Atypical Skills:** High software capability, but limited visibility into physical lab layouts, hardware stock levels, or empty port configurations.
*   **Operational Pain Points:**
    *   Filing capacity requests requires hunting down spreadsheet queues and manually checking if other internal teams have idle machines they could spare.
    *   Complete lack of visibility into delivery ETAs once a request is approved.

### 4. The Resource Planner ("The Demand Forecaster")
> *"We need to maintain capacity projections for the entire organization while preventing hardware costs from spiraling out of control."*

*   **Role Context:** Operational planner responsible for matching organization-wide forecasting models with available laboratory space and power limits.
*   **Atypical Skills:** High budget planning, spreadsheet, and resource coordination experience.
*   **Operational Focus:** Analyzing demands, matching requests to available product catalogs, and verifying if a request can be avoided by reclaiming idle fleet capacity.
*   **Operational Pain Points:**
    *   Manually calculating vacant vs. free ports and shelf space configurations using raw uncoordinated spreadsheets.

### 5. The Fulfillment Manager ("The Builder and Deliverer")
> *"Once a request is approved, my job is to translate that request into physical racks and deployed devices on the floor as fast as possible."*

*   **Role Context:** Focuses entirely on the execution and physical delivery of approved hardware requests.
*   **Operational Focus:** Coordination of physical build logistics, managing the hardware procurement queue, and tracking delivery progress.
*   **Operational Pain Points:**
    *   Manual toil involved in filing downstream deployment tickets and tracking slipped capacity requests across disconnected systems.
