# Build fix: dedupe compile errors

This bundle patches the POC connector compile errors reported during the OTel Collector Builder step:

- `invalid operation: primary == nf (struct containing siteMatch cannot be compared)`
- `existing.dedupeReason undefined`
- `cannot use rankedAgg as *agg in fillCommon`

## What changed

1. `agg` now has a `dedupeReason string` field.
2. `choosePrimary` now returns `(useNewPrimary bool, duplicate bool)` instead of returning a full `normalizedFlow` and relying on struct equality.
3. `fillCommon(...)` is called with `it.agg` when iterating ranked top conversations.
4. Suppressed exporter tracking now handles both cases:
   - existing primary wins
   - new higher-priority exporter replaces existing primary

## Rebuild

```bash
docker compose build --no-cache collector
```

Then run the POC:

```bash
docker compose --profile test up --build
```
