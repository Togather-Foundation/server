# Hardening a Togather Node

Practical host-level hardening for anyone running a Togather SEL node — written for
operators **and for the agents operating those nodes**. Every item here came out of a real
failure on a real node; each one says what went wrong, because a rule without the failure
behind it gets deleted by the next person who finds it inconvenient.

This is the host layer. For application-level security — secrets, authentication, rate
limits, database access — see [SEL Security Model](../contributors/security.md). For the
reverse proxy configuration itself see [Caddy](caddy.md) and, if something is already
broken, [Troubleshooting](troubleshooting.md).

## The two assumptions worth making

1. **Every public node is scanned, continuously, by things that lie.** Assume probes for
   `.env`, `.git`, SSH keys, database dumps and admin panels within days of your DNS
   pointing at the box. Assume the User-Agent is fiction.
2. **The failure that hurts is not the attack, it is the outage.** In the incident this
   guide is built on, a single malformed request killed the reverse proxy in under a
   second, and the endpoint stayed dead for **34 hours** — not because the bug was hard to
   fix, but because nothing restarted the service and nobody was told.

## 1. Make the reverse proxy restartable

The packaged Caddy systemd unit ships **without a `Restart=` policy**. One panic therefore
means the node serves nothing until a human notices.

```bash
sudo mkdir -p /etc/systemd/system/caddy.service.d
sudo tee /etc/systemd/system/caddy.service.d/override.conf >/dev/null <<'EOF'
[Service]
Restart=on-failure
RestartSec=3
EOF
sudo systemctl daemon-reload && sudo systemctl restart caddy
```

**Verify it, don't assume it.** Kill the process and watch systemd bring it back:

```bash
sudo kill -9 $(systemctl show caddy -p MainPID --value)
systemctl show caddy -p NRestarts    # must increment
```

Note that a manual `systemctl restart` resets `NRestarts` to 0, so read the journal too
(`Main process exited, code=killed`).

### Read the boot timeline, not just the status

"Serve nothing" and "crashed after an hour" look identical from outside. Ask the unit how
long it lived:

```bash
systemctl status caddy --no-pager -l     # note Duration: and the failure timestamp
sudo journalctl -u caddy --no-pager | grep -c panic:   # ever happened before?
```

A proxy that **started successfully and died later** is the case a restart policy fixes. A
proxy that never started is a configuration or port problem, and a restart will not help.

## 2. Keep the proxy patched, and know how to go back

Patch releases carry security and crash fixes; a fresh release can also introduce
regressions. Both happened here: the ticklish version was **two days old**, and the fix
shipped the next day.

```bash
apt-cache madison caddy                                        # what is available
sudo apt-get install -y --only-upgrade caddy                   # never a bare apt-get upgrade
sudo caddy validate --config /etc/caddy/Caddyfile && sudo systemctl restart caddy
sudo apt-get install -y caddy=<previous-version>               # rollback
```

Before blaming the app for a crash, **check the vendor's release notes for the exact
version you run**. In our incident the stack trace pointed at
`golang.org/x/net/http2 … SetReadDeadline` inside Caddy's request-body idle-timeout reader;
the release notes for the next patch said, in as many words, that the previous release
"could panic with a nil pointer dereference when the reverse proxy was still reading a
request body after the handler had returned." That turned an unbounded debugging problem
into an upgrade.

Watch out for this: `unattended-upgrades` can move the proxy between versions **without
anyone deciding to** — on our box the apt history had no record of how the installed
version got there. Silent upgrades are why the restart policy matters. If it matters which
version you run, check it:

```bash
caddy version && apt-mark showhold     # hold only with a plan to review it
```

## 3. Response hygiene: never answer 200 to a probe

Single-page apps commonly answer every unmatched path with `200` + `index.html`. That is
correct for a browser deep-linking into a client-side route, and wrong for everything else:
a scanner reads a 200 as *found something* and escalates, and your access log can no longer
distinguish a real page view from a probe.

Two denylists at the proxy cut most of it, and the durable fix is in the app: unknown
non-SPA paths should carry a **404 status** rather than a 200 with a shell page.

```caddyfile
example.org {
	# Any dot-prefixed path segment is a probe — /.env, /.git/config, /static/.env,
	# /shop/.env — plus the once-encoded /%2Eenv form. /.well-known* is carved out:
	# it is ACME and security.txt, and breaking it breaks certificate renewal.
	@probe_dotfiles {
		path_regexp dotfile (?i)(^|/)(\.|%2e)[^/]+
		not path /.well-known*
	}
	respond @probe_dotfiles 404

	# Bundler/dev-server source disclosure, with a right boundary so /@identity
	# is not caught by accident.
	@probe_tooling {
		path_regexp tooling (?i)^/@(fs|vite|id|react-refresh)(/|$)
	}
	respond @probe_tooling 404

	# Secret-bearing extensions anywhere in the path: catches /ssl/server.key and
	# /config.env, which the dotfile rule misses because the segment does not begin
	# with a dot. Exclude pre-compressed assets (.gz, .br) or you will break them.
	@probe_secrets {
		path_regexp secrets (?i)\.(env|git|ssh|htpasswd|htaccess|pem|key|sql|bak|backup|old|swp|php)$
	}
	respond @probe_secrets 404
}
```

Three things we learned the hard way:

- **`respond` is cheap and ordered before `reverse_proxy`.** Caddy evaluates directives in
  its built-in order, not in the order they appear in the file, so these blocks
  short-circuit the upstream without needing to be first in the file. The `header`
  directive runs earlier still, so your security headers do land on these 404s. Confirm
  both with `caddy adapt --config Caddyfile --pretty` rather than assuming.
- **Prefer a denylist of observed shapes over a default-deny.** A drafted rule that rejected
  *every* write method outside the API paths was dropped: it denied routes nobody had
  enumerated (admin UI, webhooks, no-JavaScript form posts), and it would not have
  prevented the crash anyway. Enumerate real traffic from your access log *before*
  restricting it.
- **Test the regex against your legitimate paths before you deploy it.** Regexes that look
  obviously right 404 something real. Our test set includes `/`, `/health`, `/admin`,
  `/api/v1/events`, `/mcp`, `/robots.txt`, `/sitemap.xml`, `/admin/static/*`,
  `/.well-known/*`, event slugs, and assets with a dot mid-segment. If you keep the proxy
  rules in version control (you should — see §7), a small test script makes this a
  one-liner.

## 4. Banning: on floods, by IP, and with modest expectations

Tools like `fail2ban` are **noise reduction, not burst defence**. A burst that matters is
over before the ban lands. Use them if you want the log quiet and repeat offenders gone,
and be honest about the limits.

- **Never key a rule on User-Agent.** Our scanner rotated roughly fifteen different
  AI-crawler identities within one second, including one that matched a major vendor's
  published format exactly. Identity in the UA string is not evidence.
- **Key on behaviour** — a flood of 404/405s from one IP — not on a hardcoded list of probe
  paths. A path list is a second copy of the config that will silently drift out of sync
  with the first.
- **Mind the log format.** JSON access logs carry no human-readable timestamp; set
  `datepattern` for the epoch field, or ban logic misfires.
- **A bad filter fails silently.** Load-test every change:

```bash
fail2ban-regex /var/log/caddy/access.log /etc/fail2ban/filter.d/<name>.conf   # does it match?
sudo fail2ban-client -t                                                       # does it load?
sudo systemctl reload fail2ban && sudo fail2ban-client status <jail>          # is it running?
```

- **Set `ignoreip`** and keep bans per-IP, never per-range. Your scanner may share a cloud
  provider with your CI, your monitoring, or a partner. And if you ever put a CDN or load
  balancer in front, the logged client address becomes the CDN's — a ban then blackholes
  the whole node. Re-point your rules at the real client IP before that day.

## 5. Size the box for what actually runs on it

A node typically runs the server, PostgreSQL, the metrics stack *and* — if the scraper
drives a headless browser per source — one or more Chromium instances. That does not fit on
a 1 vCPU / 2 GB instance, and the failure mode is ugly: the machine stops responding, the
host's watchdog reboots it, and you find out later from a log. On a Linode, a watchdog
reboot appears in the account event log as `lassie_reboot` — that is the platform
rebooting an unresponsive instance, not you. Repeated watchdog reboots are a sizing
verdict, not a mystery.

Rule of thumb: app + database + Prometheus + Grafana + headless browsers wants **at least
4 GB / 2 vCPU**, or the monitoring stack moved off the node entirely. Swap is not a fix; it
buys you minutes of thrash before the freeze.

### Reap what you spawn

Headless-browser scraping leaks child processes if the parent never reaps them. Zombies
cost no memory but they are a clean signal that the process lifecycle is wrong, and they
grow with every scrape pass:

```bash
ps -eo pid,ppid,stat,etimes,comm | awk '$3 ~ /^Z/'   # zombies
ps -p <ppid> -o comm,args                            # the parent that should reap them
```

A health check that only asks "does `/health` answer?" will not see this. Check the process
table too.

## 6. Monitor the node, not just the app

Our node exported application metrics only — no host metrics, no service state. So a dead
reverse proxy, a full disk, a zombie pile and a frozen machine were all invisible; the
outage was found by a scheduled check the next morning.

- **Probe the public endpoint from outside** the node, over TLS, on a short interval. An
  internal `curl localhost` cannot tell you that the proxy or the certificate is what died.
- **Check service state, not just ports.** `systemctl is-active caddy` catches a crash;
  port reachability catches neither a hang nor a wrong upstream.
- **Alert on the absence of information.** No metrics for an hour is a fact worth paging on,
  not a gap to ignore.
- **Scrape the host** (node_exporter or the platform's equivalent): disk, memory, load,
  process count. Every failure in this guide would have shown up there first.

## 7. Keep the running configuration in version control

The configuration on a node is often generated from a file in the repository, and the
deploy script re-syncs it. **A host-only edit is therefore temporary**: it works until the
next deploy, which silently reverts it.

- Change the source file in the repo, deploy it, and verify on the host.
- Keep a dated backup before installing anything: `sudo cp /etc/caddy/Caddyfile{,.bak-<date>}`.
- Validate before reload, and prefer `reload` over `restart` — a failed reload keeps the
  old configuration serving.
- **Verify from outside the box.** `is-active`, a listening socket and a 200 from
  `localhost` can all be true while the public endpoint is broken.

```bash
sudo caddy validate --config /etc/caddy/Caddyfile \
  && sudo systemctl reload caddy \
  && curl -sS -o /dev/null -w '%{http_code}\n' https://your-node.example.org/health
```

## 8. Notes for agents operating a node

If an agent runs the node — and increasingly one does — the useful line is between
*reversible and observable* and *the operator's call*. A workable division:

**An agent may, unasked:** read state and logs; restart a failed service; add a reversible,
reviewed drop-in; apply a patch-version upgrade; roll back its own change; verify from
outside; and report what changed.

**An agent should stop and ask:** spending money (resizing, new services); anything that
reaches another party (sends, posts, published artifacts); credential and key changes;
pinning a version deliberately; deleting logs or backups; changes that cannot be undone
with the backup it just took.

Two habits make that division safe:

- **Take a backup before every change, and say where it is.** "Reversible" is a claim about
  the backup, not about intent.
- **Verify externally after every change.** An agent that reports "restarted, looks fine"
  without a request from outside the box has reported its own optimism.

Where judgment matters more than rules — a config comment that explains *why*, honesty
about partial mitigations, or declining a change that is hardening in name only — the
standard is the same for a person or an agent: the next operator has to be able to trust
what they read here.
