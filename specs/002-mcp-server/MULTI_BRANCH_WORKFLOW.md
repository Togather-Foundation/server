# Working on Multiple Branches with OpenCode

This guide explains how to work on the MCP server (in `002-mcp-server` branch) while simultaneously working on other features in different branches.

## The Challenge

You want to:
- Build the MCP server in the `002-mcp-server` branch
- Continue other work in `main` or other feature branches
- Not have the two streams of work interfere with each other
- Keep bead tracking separate for each branch

## The Solution: Multiple Terminal/Editor Sessions

OpenCode works per-directory, so you can run multiple instances in different terminals working on different branches.

## Setup

### Option 1: Multiple Terminal Windows (Recommended)

```bash
# Terminal 1 - MCP Server Development
cd ~/Documents/Projects/Art/togather/server
git checkout -b 002-mcp-server
# Start OpenCode session for MCP work
# Work on MCP beads (server-66za, server-b33c, etc.)

# Terminal 2 - Main Branch Development
cd ~/Documents/Projects/Art/togather/server
git checkout main
# Start OpenCode session for main branch work
# Work on other beads (user admin, bug fixes, etc.)
```

### Option 2: Multiple Working Directories (Advanced)

If you want complete isolation, use git worktrees:

```bash
# Create a separate worktree for MCP development
git worktree add ../togather-mcp 002-mcp-server

# Terminal 1 - MCP work in separate directory
cd ~/Documents/Projects/Art/togather-mcp
# This is the 002-mcp-server branch in a separate directory

# Terminal 2 - Main work in original directory
cd ~/Documents/Projects/Art/togather/server
# This is your main branch
```

## Workflow

### Starting Work on MCP Server (Terminal 1)

```bash
# Switch to MCP branch
cd ~/Documents/Projects/Art/togather/server
git checkout 002-mcp-server

# Pull latest changes
git pull origin 002-mcp-server

# Check ready MCP tasks
bd ready | grep -i mcp
# Output: server-66za: Add mcp-go dependency to project

# Start work
bd update server-66za --status in_progress

# ... do work ...

# Close bead when done
bd close server-66za --reason "Added mcp-go dependency, verified compatibility"

# Commit changes
git add go.mod go.sum
git commit -m "Add mcp-go dependency"

# Sync beads state
bd sync

# Push to remote
git push origin 002-mcp-server
```

### Starting Work on Other Features (Terminal 2)

```bash
# Switch to main branch
cd ~/Documents/Projects/Art/togather/server
git checkout main

# Pull latest changes
git pull origin main

# Check ready tasks (non-MCP)
bd ready
# Output: Various non-MCP beads

# Work on a different bead
bd update server-k694 --status in_progress
# ... do work ...
bd close server-k694
bd sync
git push origin main
```

## Key Points

### Beads Are Branch-Aware
- Beads are stored in `.beads/` directory
- `bd sync` commits bead state to git
- When you switch branches, beads state switches too
- Each branch has its own bead state tracked in git

### Safe to Work in Parallel
- MCP branch adds new files (cmd/mcp-server/, internal/mcp/)
- Main branch modifies different files
- Merge conflicts are minimal (usually just go.mod)
- You can merge MCP branch to main anytime

### Syncing Beads
Always run `bd sync` before switching branches:

```bash
# Terminal 1 - MCP branch
bd sync
git push origin 002-mcp-server

# Terminal 2 - Main branch  
bd sync
git push origin main
```

### Keeping MCP Branch Updated

Periodically rebase MCP branch on main to avoid large merge conflicts:

```bash
# In MCP branch
git fetch origin
git rebase origin/main

# Resolve conflicts (if any)
git push origin 002-mcp-server --force-with-lease
```

## Example Session

### Morning: Work on MCP Server

```bash
# Terminal 1
cd ~/Documents/Projects/Art/togather/server
git checkout 002-mcp-server
git pull origin 002-mcp-server

# Work on MCP dependency setup
bd ready
bd update server-66za --status in_progress
go get github.com/mark3labs/mcp-go
# ... verify compatibility ...
git add go.mod go.sum
git commit -m "Add mcp-go dependency"
bd close server-66za --reason "Dependency added and verified"
bd sync
git push origin 002-mcp-server

# Work on MCP server infrastructure
bd ready
bd update server-b33c --status in_progress
mkdir -p internal/mcp
# ... create server.go ...
git add internal/mcp/server.go
git commit -m "Create MCP server core infrastructure"
bd close server-b33c
bd sync
git push origin 002-mcp-server
```

### Afternoon: Work on Other Features

```bash
# Terminal 2 (or same terminal, different branch)
cd ~/Documents/Projects/Art/togather/server
git checkout main
git pull origin main

# Work on user admin bug
bd ready
bd update server-k694 --status in_progress
# ... fix nullable parameter bug ...
git add internal/storage/postgres/queries/feeds.sql
make sqlc
git add internal/storage/postgres/feeds.sql.go
git commit -m "Fix nullable parameter bug in ListEventChanges"
bd close server-k694
bd sync
git push origin main
```

## Merging MCP Branch to Main

When MCP server is complete:

```bash
# Make sure both branches are synced
git checkout 002-mcp-server
bd sync
git push origin 002-mcp-server

git checkout main
bd sync
git push origin main

# Merge MCP branch
git merge 002-mcp-server

# Resolve conflicts (likely only go.mod)
# ... resolve ...
git add go.mod go.sum
git commit -m "Merge MCP server implementation"

# Run full test suite
make ci

# Push merged main
git push origin main

# Delete merged branch
git branch -d 002-mcp-server
git push origin --delete 002-mcp-server
```

## Troubleshooting

### "bd: command not found"
Make sure beads CLI is installed: `go install github.com/togather-foundation/bd@latest`

### Beads out of sync between branches
```bash
# Force sync from git
git checkout 002-mcp-server
bd sync --status
# This shows if beads are in sync with git
```

### Merge conflicts in .beads/ directory
```bash
# Use the version from the branch you're merging into
git checkout --ours .beads/
# Or use the version from the branch you're merging
git checkout --theirs .beads/
# Then re-sync
bd sync
```

### Changes not showing in beads
```bash
# Verify beads are being tracked
bd list --status open | grep server-66za
# If not found, check git log for bead commits
git log --oneline | grep beads
```

## Best Practices

1. **Sync Often**: Run `bd sync` after closing beads
2. **Commit Frequently**: Small commits are easier to merge
3. **Push Regularly**: Share your progress with the team
4. **Rebase Periodically**: Keep MCP branch updated with main
5. **Use Descriptive Commits**: Makes merge conflicts easier to resolve
6. **Test Before Merge**: Run `make ci` on merged branch

## Summary

- ✅ Use separate terminals for different branches
- ✅ Run `bd sync` before switching branches
- ✅ MCP branch is additive (minimal conflicts)
- ✅ Merge anytime without blocking other work
- ✅ Beads track independently per branch

---

**Need Help?**
- Check `bd --help` for command reference
- View beads: `bd list --status open`
- Check dependencies: `bd show <bead-id>`
- See what's ready: `bd ready`
