# Review handoff and waiting

Use board `togather` explicitly. Read live `kanban_help` for tool arguments,
claim lifetime and lifecycle behavior; this guide records the workflow and
communication policy rather than a copy of the entire tool reference.

## Hand off a reviewable revision

Claim the ticket before editing. Finish the authorized implementation,
documentation and relevant checks, then push the feature branch. Record the
repository, branch, full SHA, changed scope and check evidence on the ticket.
For shared branches, identify the commit slice each ticket owns.

Use `ticket_request_review` with both board and ticket ID, an explicit
non-dispatchable implementer `worker` label, a verified installed `reviewer`
profile, and the summary. `force: true` releases a running claim: use it only
for your own handoff. The implementer label must not be an installed profile;
otherwise a changes request can spawn another worker instead of returning to you.
Never invent a reviewer name; use the configured, verified profile.

`ticket_complete` can perform the review handoff using server defaults. If it
refuses because no implementer or reviewer resolves, use explicit
`ticket_request_review` arguments. Operators should validate `MCP_CLAIM_WORKER`
and `MCP_REVIEWER_PROFILE` against these roles during bridge setup. Remote MCP
server environment is not configured by guessing settings in the client's TOML.

Claims report their actual expiry and requested TTL (currently a four-hour
default). Do not renew or reclaim another worker's running ticket. `triage` is
internal intake, not an accepted `ticket_create` option.

## Interpret the verdict

| State or event | Required action |
|---|---|
| `review` | Handoff is queued; preserve continuation state. |
| `running` after review handoff | Reviewer may be working; no implementation re-claim. |
| Heartbeat, owner admission, timeout, unchanged status | Bookkeeping; no user-facing update or merge authorization. |
| `done` after approval | Verify the approved revision and merge/push it after all branch tickets clear. |
| Changes requested, returned to implementer | Read the full verdict, re-claim when claimable, fix and request another review. `todo` may still have dependencies. |
| `blocked` | Inspect and surface the actual blocker once; do not treat it as normal review waiting. |

Reviewers approve; implementers merge. A comment containing an approval is not
the completed lifecycle transition: wait for the ticket's verdict state. Never
merge a newer unreviewed code revision on the strength of an older approval.

## Quiet waiting

Announce the review handoff once. Further messages should convey an actionable
verdict or blocker, meaningful new evidence, or answer the user's status request.
Do not narrate repeated polls, heartbeats, timeouts, unchanged review status,
"ready as soon as approved", or the fact that nothing else remains.

Prefer a supported durable verdict subscription that suspends the task and
resumes it on the current review's actionable outcome. Persist enough context
to continue safely: board and ticket IDs, review epoch/run when available,
repository and branch, pushed SHA, last event cursor, remaining branch approvals,
and pending merge/push/branch-cleanup obligations. A resumed task must reconcile
current state and reject stale approvals or duplicate continuations.

If no such integration is available, `ticket_events` supports cursored
long-polling. It also returns for non-verdict events and status changes; these
returns do not justify a chat update. Preserve the latest observed cursor and
inspect truncated results before advancing past unseen events. This fallback
still requires model turns; it is not a durable zero-turn continuation. State
that limitation once when relevant. Higher-priority host requirements remain
in force; repository instructions cannot add runtime capabilities or override
those requirements.

Ticket `t_4cdbdae8` tracks durable zero-model-turn verdict waiting. Do not claim it
is delivered by this instruction update. Suspending review does not cancel the
authorized merge and cleanup work or make the delivery complete; preserve that
obligation and report pending state honestly if continuation is unavailable.

## Cold-agent walkthrough

- **Approval:** pushed SHA A enters review, reviewer-start/heartbeat events arrive,
  then the current review completes. Announce handoff once; reconcile approval
  for A, verify every shared-branch ticket, merge/push and clean up.
- **Changes:** reviewer returns corrections to the implementer. Read the full
  verdict, claim when dependencies permit, fix and push SHA B, and request a new
  review. Approval of A cannot authorize B.
- **Blocker:** inspect a real blocked verdict, report the actionable reason once
  and preserve continuation context. Additional heartbeats/timeouts do not
  produce repeated messages or imply the blocker cleared.
