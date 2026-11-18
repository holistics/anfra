# Conversions anfra does not support yet

Each page here converts a LookML construct to an AML feature that Holistics has and anfra does not run yet. The
conversion drops these constructs and reports them; the matching page in [references/](../references/) says how.

When anfra supports a feature, move its page back to `references/`, replace the short page there, and make
`scripts/lib/` emit the AML again.

| Page | Holistics feature | Short page |
|---|---|---|
| [aggregate-awareness.md](aggregate-awareness.md) | `PreAggregate`, `PreAggregateSchedule` | [references/aggregate-awareness.md](../references/aggregate-awareness.md) |
| [derived-table-persistence.md](derived-table-persistence.md) | query model `persistence`, `Schedule` | [references/derived-tables.md](../references/derived-tables.md#persistence-parameters) |
