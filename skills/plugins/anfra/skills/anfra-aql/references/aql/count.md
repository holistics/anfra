## Description

Counts the number of non-null values on a column expression in a table.

## Syntax

```aql
table | count(column)
```

### Input
* `table`: Source Table
* `column`: A column expression on the given table

### Output
`Scalar(number)`

### Examples
```aql
orders | count(orders.id)
cities | count(orders.id)
```

### Empty groups
A group with no rows to count gives `null`, not `0`. This happens when `where()` removes every row of the group, or when the group comes from another model and has no matching rows.

* If every measure in an explore row is `null`, the explore drops the row.
* With no dimensions, `count` over no rows gives `0`.
* Use `x | coalesce(0)` when you need `0`. Use [`count_if`](count_if.md) when the group has rows and you want `0` for "none match".

```aql
// null for a user with orders but none cancelled
orders | count(orders.id) | where(orders.status == 'cancelled')

// 0 for a user with orders but none cancelled
orders | count(orders.id) | where(orders.status == 'cancelled') | coalesce(0)
orders | count_if(orders.status == 'cancelled')
```

See [Empty groups give null, not 0](../aqlearn.md#empty-groups-give-null-not-0).
