# Anfra

anfra queries the Datasets in a Repo, and with Data App serving it also lists its Data Apps and renders the selected one against live data.

## Language

### Inputs

**Repo**:
A directory holding AML models and datasets, the anfra data source config, and Data Apps. It is anfra's only input; any directory with this layout works.
_Avoid_: Data Folder, data repo, project, workspace

**Dataset**:
An AML-defined set of models, relationships and metrics that a query runs against, addressed by its name.
_Avoid_: data set, table

**Data Source**:
A named database connection declared in the Repo's anfra config.
_Avoid_: connection, database

**Data App**:
A single HTML file in the Repo that uses the Anfra SDK to query Datasets and render the results.
_Avoid_: app, dashboard, report

### Runtime

**Data App serving**:
The part of anfra that serves a Repo's Data Apps to a browser, through the Shell. It is opt-in when anfra starts serving.
_Avoid_: demo, anfra demo, app server

**Shell**:
The Data App serving UI: a list of Data Apps on the left, and the selected Data App running on the right, isolated from the Shell.
_Avoid_: host page, portal, gallery

**Data App URL**:
The address of a Data App in the browser: its path under the Repo's `apps/` without the `.html` extension. Only Data Apps have one; folders do not.
_Avoid_: route, deep link

**Reserved namespace**:
The part of the Shell's address space that belongs to Data App serving itself, never to a Data App. A Data App whose Data App URL would fall inside it is left out of the Shell.
_Avoid_: internal routes, system paths

**Anfra SDK**:
The authoring library a Data App uses to declare queries, controls and filters. A deliberate fork of the Holistics Data App SDK. Its own language (Backend, Provisioner, …) is in `sdk/CONTEXT.md`; the Shell is its Provisioner.
_Avoid_: data sdk, data-app-sdk, Holistics SDK

### Queries

**Query Input**:
The structured additions a Data App's query carries on each run (control filters, cross-filter conditions, sorts and date drills), which are applied by rewriting the query's AQL before it compiles.
_Avoid_: query params, overrides, runtime conditions

**Date Drill**:
A Query Input entry that redraws every dimension of the query built directly on a date field at a chosen grain, keeping the dimension's alias. It changes what a dimension shows, never which rows are included, so it is not a filter.
_Avoid_: date filter, transform_date_drill

**Execution Options**:
Per-run settings that are not AQL (page, page size and timezone), applied when the query is compiled rather than by rewriting it.
_Avoid_: query input, pagination params

**Executed AQL**:
The AQL that actually ran: the author's AQL with the Query Input applied, printed back as text so the run can be inspected.
_Avoid_: final AQL, rewritten query
