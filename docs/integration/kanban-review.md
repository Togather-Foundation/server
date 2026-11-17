<!-- AGENT:NAV
purpose:~review;reviewer;branch;ticket;default;detail;handoff;revision
lines:93
nav[5]{s,n,name,about}:
12,82,#Review handoff and waiting,~review;reviewer;branch;ticket;default
18,13,##Hand off a reviewable revision,~branch;commit;complete;review;revision
31,9,##Interpret the verdict,~merge;revision;approval;branch;pushed
40,14,##Quiet waiting,~review;approvals;branch;events;pending
54,40,##Optional Hermes kanban integration,~reviewer;ticket;review;default;detail
-->

# Review handoff and waiting

Togather does not require a particular issue tracker, review service or MCP server.
Use the tools configured for the contributor or session. The filename is retained
for existing links; the review workflow applies with or without a kanban board.

## Hand off a reviewable revision

Coordinate task ownership before editing. Finish the authorized implementation,
documentation and relevant checks, then push the feature branch. Include repository,
branch, full SHA, changed scope and check evidence in the review request. For shared
branches, identify the commit slice each work item owns. If the branch was already
merged/deleted, name the merge commit and diff range so reviewers can find the work.

Use the configured review process. Read complete revision instructions and verdicts;
inspect truncation indicators rather than treating clipped comments as complete.
If required tooling or configuration is missing, report the specific blocker instead
of inventing reviewer identities, unsupported arguments or alternate lifecycle steps.

## Interpret the verdict

A queued or running review is not approval. Address requested changes and submit
the new pushed revision. Merge only the approved revision once all reviews sharing
the branch clear, using the merge authority established for the current session.
An older approval does not authorize a newer unreviewed revision. Update any tracker
and remove the merged branch after the merge is pushed. Report actual blockers
separately from ordinary review waiting.

## Quiet waiting

Announce the review handoff once. Further updates should convey actionable verdicts,
blockers, meaningful evidence or requested status. Avoid narrating heartbeats,
timeouts and unchanged status. Prefer supported notifications; otherwise use
cursored polling. Polling still requires model turns and is not a durable
subscription. Honor host communication and execution limits.

Preserve the review identity, repository/branch, pushed SHA, last event cursor,
remaining branch approvals and pending merge/push/cleanup obligations across
suspension. Reconcile current state on resume and reject stale approvals. On
truncated events, retrieve missing detail before advancing past unseen events.
A pending review does not make delivery complete.

## Optional Hermes kanban integration

These notes apply only when a contributor has configured Hermes. Board names,
implementer labels, reviewer profiles and credentials belong in that contributor's
setup; none is prescribed by Togather. Read live `kanban_help` and exposed tool
schemas for the current contract.

- Supply the configured board on board-scoped/per-ticket calls and retain returned
  ticket IDs. Boardless discovery tools use their exposed schemas. Avoid `default`
  as an assignee: it is the host worker profile. Route follow-ups to the proper lane.
- Claim before editing. Claims default to four hours and there is no heartbeat verb;
  expiry requeues the card and records a failed attempt. Reclaim only when claimable,
  never while a worker/reviewer holds it. `todo` waits on parents, `scheduled` on time,
  and `triage` is internal intake. No promote verb or `triage: true` creation flag exists.
- Push before `ticket_request_review`. Supply a non-dispatchable implementer `worker`
  (or configured `MCP_CLAIM_WORKER`) and a summary with refs and evidence. Normally
  omit `reviewer`: resolution is explicit override → `MCP_REVIEWER_PROFILE` → the last
  changes-requested round's reviewer. The server verifies installed profiles. Report
  unresolved reviewer configuration to the operator rather than guessing or retrying.
  Use `force: true` only for your own running handoff; omit it for ready tickets.
- Native handoff enters `review`; the reviewer may move it to `running`. Wait for the
  verdict: approval completes to `done`; changes return to the implementer. `blocked`
  is an actual blocker. Direct completion is not review approval: `ticket_complete`
  requires a running claim by default; LOW completes directly, MEDIUM/HIGH (default
  MEDIUM) use review, and `MCP_COMPLETE_MODE=done` overrides that handoff. Use native
  review when the change requires review. Completion accepts repo/branch/sha, caps
  handoff summaries at 16 KiB and verifies `created_cards` child IDs in done mode.
- Read steers, revision comments and verdicts with `ticket_get(detail="full")`.
  Partial detail clips comments; full detail is also bounded (32 KiB). Inspect
  truncation and dropped-comment metadata. Use only options exposed by the client;
  report capability gaps that prevent reading required content.
- Wait with `ticket_events` using the last event ID as `since_event_id`.
  `wake_on="verdicts"` is the default (omit if absent from the client schema): it
  suppresses bookkeeping and reviewer moves into running/review. `wake_on="any"`
  restores unfiltered behavior. Check `ticket_status` even when no events return.
  `suppressed` counts bookkeeping since the cursor, so repeated cursors repeat the
  counts. Truncated events require detail retrieval before advancing. A cursor
  already past a completed verdict waits out the timeout; reconcile ticket detail
  on resume. The default timeout is 120 seconds, maximum 900; choose shorter waits
  or yield execution to satisfy host limits. This is stateless long-polling.
