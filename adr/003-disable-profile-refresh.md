# ADR 003: Disable Mobalytics Profile Refresh

## Status

**Accepted** - 2026-05-04

## Context

The League Watcher Bot calls `RefreshProfile()` via WebSocket to Mobalytics GraphQL API before fetching match history for each player. This was intended to ensure the profile cache is up-to-date on Mobalytics' side before querying for recent matches.

### Implementation Details

- **WebSocket endpoint**: `wss://ws.mobalytics.gg/api/lol/graphql/v1/query`
- **Subscription**: `LolSummonerUpdateSubscription`
- **Implementation**: `internal/leaguewatcher/watcher/mobalytics/client.go:333-384`
- **Call site**: `internal/leaguewatcher/watcher/watcher.go:89-95`

### Current Problem

**The Mobalytics API has changed, breaking the current implementation:**

1. Profile refresh calls consistently fail during polling cycles
2. Failures generate error spam in logs (warning messages for each player, every poll period)
3. Match detection continues to work normally without the refresh step
4. Test coverage for this feature is already broken (`TestRefreshProfiles` marked as "todo fix this test")

The bot's match detection functionality has been working reliably despite refresh failures, indicating that:
- Either Mobalytics auto-refreshes profiles without explicit requests
- Or the refresh was never strictly necessary for match detection
- The failure is purely in the WebSocket subscription mechanism, not the underlying data

### Why Fix This Now

1. **Log pollution**: Warning messages clutter logs, making it hard to spot real issues
2. **Performance**: Failed WebSocket connections add latency to each polling cycle (30-second timeouts)
3. **Broken API surface**: Keeping broken code active is confusing for future maintainers
4. **Clear intent**: Commenting out broken code signals it needs investigation vs. "it's supposed to fail sometimes"

## Decision

**Temporarily disable the profile refresh functionality** by commenting out the calls in `watcher.go:checkPlayers()` until the new Mobalytics API can be investigated and the implementation updated.

**Approach:**
1. Comment out lines 89-95 in `watcher.go` (the refresh call block)
2. Add TODO comment referencing this ADR
3. Preserve original code for reference when fixing
4. Document decision in this ADR for future developers

**This is a temporary measure, not a permanent removal.** The RefreshProfile implementation in `client.go` remains unchanged for reference.

## Consequences

### Positive

1. **Eliminates error spam**: No more "failed to refresh" warnings cluttering logs
2. **Match detection continues**: Functionality unaffected, matches still detected reliably
3. **Reduced polling latency**: Removes 30-second timeout overhead for failed WebSocket connections
4. **Preserves code**: Original implementation kept for reference when fixing
5. **Clear intent**: TODO comment signals temporary disable, not abandoned feature
6. **Easier debugging**: Real errors in logs are no longer buried in refresh failures

### Negative

1. **Mobalytics profiles may not be as fresh**: Profiles queried without explicit refresh request
   - **Mitigation**: Match detection has been working fine without it
2. **May affect match detection**: If Mobalytics requires explicit refresh for recent matches
   - **Mitigation**: Monitor match detection after deployment; can revert if issues arise
3. **Technical debt**: Adds TODO that needs addressing later
   - **Mitigation**: Clearly documented in ADR with plan for resolution

### Neutral

1. **Need to investigate new Mobalytics API**: WebSocket/GraphQL subscription structure may have changed
2. **Test coverage already broken**: `TestRefreshProfiles()` needs fixing regardless
3. **Implementation preserved**: Can re-enable once API is understood

## Alternatives Considered

### Alternative 1: Complete Removal of RefreshProfile Function

**Approach**: Delete `RefreshProfile()` from `mobalytics/client.go` entirely, remove all references.

**Pros**:
- Cleaner codebase, no dead code
- Forces proper investigation when re-implementing
- Clear signal that feature is gone

**Cons**:
- **Loses reference implementation**: No baseline when investigating new API
- **More work to restore**: Must recreate from scratch vs. uncomment
- **Git archaeology required**: Would need to dig through history to find original implementation
- **Harder to compare**: Can't compare old vs. new API side-by-side

**Rejection reason**: Preserving the implementation provides valuable reference for fixing the API integration. Commenting out is safer and more reversible.

### Alternative 2: Add Configuration Flag

**Approach**: Add `disable_profile_refresh` boolean to `config.yaml`, make refresh conditional.

**Pros**:
- Users can toggle without code changes
- More flexible for testing
- No code comments needed

**Cons**:
- **This is a broken feature, not an optional one**: Not a user-facing configuration choice
- **Over-engineering**: Adds config complexity for temporary issue
- **Confusing intent**: Implies feature is optional, not broken
- **User burden**: Requires users to know about broken feature and set config

**Rejection reason**: This isn't a feature users should control; it's broken infrastructure that needs fixing. Configuration is wrong abstraction.

### Alternative 3: Keep Calling but Suppress Errors

**Approach**: Catch errors silently without logging warnings.

**Pros**:
- No log pollution
- Preserves original code flow
- Easy to revert

**Cons**:
- **Wastes resources**: Still makes failing WebSocket connections (30s timeout per player)
- **Silent failures**: Hides that feature is broken
- **Performance impact**: Adds latency to every poll cycle for no benefit
- **Confusing to maintainers**: Why is code silently failing?

**Rejection reason**: Worse than commenting out - burns resources on broken API calls while hiding the problem.

### Alternative 4: Implement Retry Logic

**Approach**: Add retry mechanism with exponential backoff for refresh failures.

**Pros**:
- Might work if API is intermittently available
- Standard error-handling pattern

**Cons**:
- **API is not intermittent, it's broken**: Retry won't help
- **Wastes more resources**: Multiple failed attempts per poll cycle
- **Complexity**: Adds code to broken feature
- **Wrong problem**: Need to fix API integration, not retry harder

**Rejection reason**: Retrying a broken API integration is throwing good money after bad. Fix the root cause instead.

## Future Work

### Phase 1: Investigation (Before Re-enabling)

1. **Investigate new Mobalytics GraphQL API structure**
   - Check if WebSocket endpoint changed
   - Verify subscription query format
   - Test if profile refresh is still necessary
   - Document findings

2. **Understand refresh necessity**
   - Monitor match detection without refresh for 1-2 weeks
   - Verify no missed matches or stale data issues
   - Determine if Mobalytics auto-refreshes profiles
   - Consider removing feature entirely if unnecessary

### Phase 2: Implementation (If Re-enabling)

3. **Update `RefreshProfile()` implementation**
   - Modify GraphQL subscription to match new API
   - Update WebSocket connection parameters if needed
   - Fix error handling and timeout logic
   - Add integration tests

4. **Fix broken test `TestRefreshProfiles()`**
   - Update test to match new API structure
   - Remove "todo fix this test" skip
   - Add test cases for error scenarios
   - Ensure test is reliable

5. **Validate match detection reliability**
   - Deploy to staging with refresh enabled
   - Monitor for 24-48 hours
   - Compare match detection with/without refresh
   - Verify no performance regression

### Phase 3: Decision Point

6. **Decide: Re-enable or Remove**
   - If refresh improves match detection: Re-enable with updated implementation
   - If no measurable benefit: Delete `RefreshProfile()` entirely and update this ADR
   - Document decision and rationale in new ADR or update this one

## Monitoring and Validation

### After Disabling Refresh

**Watch for**:
- Missed matches (players report games not detected)
- Delayed match notifications (games detected later than expected)
- Stale profile data (wrong rank, champion pool, etc.)

**If issues occur**:
1. Check `watcher.go` logs for "failed to get matches" errors (unrelated to refresh)
2. Verify Mobalytics API availability (check website)
3. Consider re-enabling refresh as temporary workaround
4. Escalate to "Future Work Phase 1" investigation immediately

**Success criteria** (for keeping refresh disabled):
- 2 weeks of production use with no match detection issues
- No user reports of missed/delayed games
- Logs show clean polling cycles without errors

## Implementation Checklist

- [x] Comment out refresh calls in `watcher.go:89-95`
- [x] Add TODO comment referencing ADR-003
- [x] Create this ADR document
- [x] Update AGENTS.md with ADR documentation expectations
- [ ] Run tests to ensure no regressions
- [ ] Deploy to production
- [ ] Monitor for 2 weeks
- [ ] Decide on permanent fix (re-enable or remove)

## Technical Details

### Code Changes

**File**: `internal/leaguewatcher/watcher/watcher.go`

**Before** (lines 89-95):
```go
w.logger.Debug("refreshing player", zap.String("player", player.Name))
status, err := w.api.RefreshProfile(ctx, player.Region, player.Name, player.Tag)
if err != nil {
    w.logger.Warn("failed to refresh", zap.String("player", player.Name), zap.Error(err))
} else {
    w.logger.Debug("refreshed", zap.String("player", player.Name), zap.String("status", status))
}
```

**After**:
```go
// TODO: Profile refresh disabled - Mobalytics API changed, needs investigation
// See ADR-003 for details and plan to re-enable
// w.logger.Debug("refreshing player", zap.String("player", player.Name))
// status, err := w.api.RefreshProfile(ctx, player.Region, player.Name, player.Tag)
// if err != nil {
//     w.logger.Warn("failed to refresh", zap.String("player", player.Name), zap.Error(err))
// } else {
//     w.logger.Debug("refreshed", zap.String("player", player.Name), zap.String("status", status))
// }
```

**Unchanged**: 
- `RefreshProfile()` implementation in `client.go` (preserved for reference)
- Test file `client_test.go` (already has skipped test)
- Match fetching logic (lines 97+) continues as normal

### Impact on Polling Cycle

**Before** (per player, per poll):
1. Establish WebSocket connection (0-5s)
2. Subscribe to profile updates (0-2s)
3. Wait for response or timeout (0-30s if failing)
4. Log result
5. Fetch matches via HTTP (0-5s)
6. Process and notify

**After** (per player, per poll):
1. ~~Establish WebSocket connection~~ (skipped)
2. ~~Subscribe to profile updates~~ (skipped)
3. ~~Wait for response or timeout~~ (skipped)
4. ~~Log result~~ (skipped)
5. Fetch matches via HTTP (0-5s)
6. Process and notify

**Performance improvement**: Removes 0-35 seconds per player per poll cycle (more if failures are common).

For 5 players with 1-minute poll period:
- **Before**: 0-175s wasted on failed refreshes per cycle (potentially exceeds poll period!)
- **After**: 0s wasted on refreshes

## References

### Code Locations

- **RefreshProfile implementation**: `internal/leaguewatcher/watcher/mobalytics/client.go:333-384`
- **Call site (disabled)**: `internal/leaguewatcher/watcher/watcher.go:89-95`
- **Broken test**: `internal/leaguewatcher/watcher/mobalytics/client_test.go:53-94`
- **GraphQL subscription query**: `internal/leaguewatcher/watcher/mobalytics/client.go:314-331`

### Related ADRs

- [ADR 001: Docker Containerization](./001-docker-containerization.md) - Deployment strategy
- [ADR 002: CI/CD Docker Pipeline](./002-ci-cd-docker-pipeline.md) - Automated builds and versioning

### External Resources

- Mobalytics GraphQL API endpoint: `wss://ws.mobalytics.gg/api/lol/graphql/v1/query`
- GraphQL subscription: `LolSummonerUpdateSubscription`
- Mobalytics website: https://mobalytics.gg/

## Reviewers

- **ADR Author**: Claude (AI Assistant)
- **Approved By**: v.loginov
- **Date**: 2026-05-04

## Changelog

- **2026-05-04**: Initial ADR documenting decision to disable broken profile refresh feature
  - Mobalytics API changed, breaking WebSocket subscription
  - Profile refresh calls commented out in watcher.go
  - Implementation preserved for future reference
  - Match detection confirmed working without refresh
