---
name: agentmap-maintain
description: Read and maintain AGENT:NAV blocks in Markdown files.
---
<!-- AGENT:NAV
purpose:~nav;files;agentmap;block;read;agent;lines;markdown
lines:29
-->

<!-- agentmap:init -->
## Reading Markdown Files

AGENT:NAV placement and shape:

- AGENT:NAV appears immediately after any frontmatter.
- Files over 50 lines get a nav block with section entries.
- Files under 50 lines get a purpose-only block (and may include `see`).

Read the first 50 lines first then use the nav block to jump to the section you need.

- If purpose doesn't match your task; stop reading.
- Use s;n line ranges: Read(offset=s; limit=n) for the section you need.
- If a description has `>` hints (e.g. `topic>sub1;sub2;sub3`); scan the hints to find the right subsection before reading the whole parent section.
- Check see before searching; the file you need may be listed.
- If line numbers seem off; grep for the heading as fallback.

## Before Committing Markdown Changes

1. Run: agentmap update <changed files>
2. Review output for sections marked content-changed or new.
3. Read flagged sections and update their descriptions in the nav block.
4. Commit.
<!-- /agentmap:init -->
