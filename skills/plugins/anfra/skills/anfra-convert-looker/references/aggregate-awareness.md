# LookML `aggregate_table`

Looker: [`aggregate_table`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table),
[Aggregate awareness](https://docs.cloud.google.com/looker/docs/aggregate_awareness)

**Unsupported.** anfra has no pre-aggregates. Drop every `aggregate_table` of an explore, with all its parameters
(`query`, `materialization`), and list it in the conversion report with its original LookML.

The script does this: each aggregate table is a `DROPPED` note in `work-items.json`, with the LookML. It leaves no
comment in the Dataset.

- **Results do not change.** An aggregate table only makes Looker's queries faster. Without it, every query runs on
  the source tables and returns the same numbers.
- **Queries can be slower.** Report the aggregate tables of the explores whose source tables are large.
- Do not write a `PreAggregate` or a `PreAggregateSchedule`. `anfra validate` accepts them, but anfra never builds
  or uses them.
- Parameters that only serve aggregate tables are dropped too: a measure's `allow_approximate_optimization`
  (see [measure.md](measure.md#allow_approximate_optimization)).
