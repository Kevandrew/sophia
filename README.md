<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/branding/logo.svg">
    <source media="(prefers-color-scheme: light)" srcset="docs/assets/branding/logo-light.svg">
    <img src="docs/assets/branding/logo-light.svg" alt="Sophia logo" width="110">
  </picture>
</p>
<h1 align="center">Sophia</h1>
<p align="center"><strong>Your AI builds the code. Sophia makes sure it's right.</strong></p>
<p align="center"><a href="https://sophiahq.com/blog/at-what-point-do-we-stop-reading-code/">Read the manifesto: At what point do we stop reading the code?</a></p>



---

AI writes code fast. But it doesn't keep track of *why* it made each change, *what* it was allowed to touch, or *how to undo it* when something breaks.

Half the code shipping today was pushed straight to prod by someone spawning AI agents from their phone while stuck in traffic. The other half is sitting in a pull request no one has time to read.

Sophia fixes both problems.

## What Sophia is

Sophia is a layer between your idea and the finished code.

You describe what you want. Before anything gets built, Sophia writes down:

- **What's changing** and **why**
- **What's off-limits** — the parts of your project that shouldn't be touched
- **What "done" looks like** — how to know the change actually worked
- **How to undo it** — so you're never stuck

Then your AI builds against that record. When it's done, Sophia checks: *did the code stay within the lines?* If it drifted — you see exactly where.

Every change gets a clear trail. Not buried in chat logs or commit messages — a structured record you can trace back to when something breaks.

## How it works

```
You: "Add dark mode to settings. Only touch the frontend.
      Don't break the existing light theme."

Sophia:  ✓ Change request created
         ✓ Scope: frontend only — must not break light theme
         ✓ Broken into 3 tasks, each with clear success criteria
         ✓ AI builds each task against the contract
         ✓ Validated — no files outside frontend were changed
         ✓ Merged
```

No commands to learn. Your agent does the work. Sophia keeps it honest.

## Just install the skill

Sophia ships as an **agent skill**. Install it once, and your AI already knows how to use it — no setup, no learning curve.

> **Install the Sophia skill:**
> - [Agent Quickstart](docs/agent-quickstart.md) — install via `https://sophiahq.com/install.sh | bash`, then install the in-repo skill
> - [Skill file](skills/sophia/SKILL.md) — source artifact to install into your agent skills directory

Keep talking to your agent the way you always do. Sophia runs underneath.

## Why it matters

**If you're building solo** — Sophia is your witness. It was there when every change was made. It recorded what changed, why, and what it was allowed to touch. When something breaks at 2am, you don't scroll through chat history — you ask Sophia *why* and it tells you.

**If you're on a team** — Sophia replaces the pull request bottleneck. Instead of waiting days for someone to review a diff they don't have context for, the review is built into the process. Contracts are set before the code is written. Verification is automatic. Your senior engineers stop reading diffs and start designing intent.

## The big idea

AI writes code faster than humans can review it. That gap — between what gets written and what gets trusted — is where bugs, regressions, and wasted time live.

Sophia closes the gap. You review *what the code was supposed to do*, not what it looks like. Trust is earned through evidence, not eyeballs.

**Code is the output. Intent is the artifact.**

## Get started

```bash
curl -fsSL https://sophiahq.com/install.sh | bash
```

Then follow the [Agent Quickstart](docs/agent-quickstart.md) for skill install and first prompts.

<details>
<summary>Full CLI for power users</summary>

Sophia has a complete command-line interface for direct control. See the [CLI Reference](docs/cli-reference.md) or run `sophia --help`.

</details>

## Collaboration Model

Sophia remains local-first and network-agnostic. Collaboration works through portable artifacts:

- `sophia cr export <id>` emits a canonical bundle (`sophia.cr_bundle.v1`) including CR identity and fingerprint.
- `sophia cr import --file <bundle.json> --mode create|replace` materializes shared CR state locally.
- `sophia cr patch apply <selector> --file <patch.json>` applies structured suggestions with conflict detection.
- `sophia cr patch preview <selector> --file <patch.json> --json` checks patch compatibility without mutating state.

This keeps the core CLI MIT and platform-agnostic. A separate team platform/tool can own auth, storage, and collaboration UX while calling Sophia locally.

## What's next

**Today** — Sophia runs locally. No accounts, no servers. Everything stays in your repo.

**Sophia HQ** (coming soon) — managed collaboration built on top of Sophia artifacts (bundle + patch) for shared CR review and orchestration.

## Learn more

[Agent Quickstart](docs/agent-quickstart.md) · [Getting Started](docs/getting-started.md) · [Workflow](docs/workflow.md) · [CLI Reference](docs/cli-reference.md)


## Community

[Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Code of Conduct](CODE_OF_CONDUCT.md) · [License](LICENSE)

---

<p align="center"><em>Your AI writes the code. Sophia makes sure it's right.</em></p>
