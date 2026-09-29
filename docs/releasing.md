# Releasing

One git tag publishes everything:

| What | Where | How people use it |
| --- | --- | --- |
| GitHub Action | GitHub Marketplace ("LGTM Review Gate") | `uses: donaldwasserman/lgtm@v1` |
| Command-line binaries | [Releases page](https://github.com/donaldwasserman/lgtm/releases) | download and run |
| Docker image | `ghcr.io/donaldwasserman/lgtm` | `docker run ghcr.io/donaldwasserman/lgtm:1` |
| Go install | Go module proxy | `go install github.com/donaldwasserman/lgtm/cmd/lgtm@latest` |

## Version numbers

Versions look like `v1.4.2` (major.minor.patch).

- **Patch** (`v1.4.2 → v1.4.3`): bug fixes, same results.
- **Minor** (`v1.4 → v1.5`): new options or languages; existing setups behave the same.
- **Major** (`v1 → v2`): anything that can break someone's setup or flip a result:
  - removing or renaming an input, output, or flag
  - changing a default (thresholds, `check-name`, `trust-mode`)
  - changing the JSON report or exit codes
  - changing how an existing language is measured

Each release has an exact tag (`v1.4.2`) that never moves, and a major tag
(`v1`) that always points at the newest `v1.x.y`. Docker images use the same
numbers without the `v`: `1.4.2`, `1.4`, `1`, `latest`.

## How to release

1. Make sure `main` is green.
2. In one pull request: set `VERSION` to the new number (e.g. `1.4.3`), and
   add a `## 1.4.3` section at the top of `CHANGELOG.md`. Merge it.
3. Tag the merge commit and push the tag:
   ```bash
   git checkout main && git pull
   git tag v1.4.3 && git push origin v1.4.3
   ```
4. Wait for the **release** workflow. It checks the tag matches `VERSION`,
   runs the tests, builds binaries, and creates a **draft** release.
5. On GitHub, open the draft release. Check the notes, tick **"Publish this
   Action to the GitHub Marketplace"**, and click **Publish**.
6. Publishing starts the **publish** workflow, which pushes the Docker image
   and moves the `v1` tag. Once it's green, the release is live.
7. Try it: `uses: donaldwasserman/lgtm@v1.4.3` in a test repository, and
   `docker run --rm ghcr.io/donaldwasserman/lgtm:1.4.3 --version`.

Why the manual step: GitHub only lets a person publish to the Marketplace, so
the workflow stops at a draft. Nothing is public until you click Publish.

**If a release is broken:** don't delete or move its tag. Release a fix as the
next patch version; `v1` follows automatically.

**If the release workflow fails:** fix the problem on `main`, delete the tag
(`git push --delete origin v1.4.3 && git tag -d v1.4.3`) and any draft it
made, then tag again. This is safe only while the release is still a draft.

## What happens behind the scenes

| Workflow | Runs when | Does |
| --- | --- | --- |
| `ci.yml` | every push to `main` and pull request | tests, model checks, Docker build (not pushed), runs the action on itself |
| `release.yml` | a `v1.2.3` tag is pushed | checks, builds Linux and macOS binaries (amd64 + arm64), creates a draft release with checksums |
| `publish.yml` | a release is published | pushes the Docker image, moves the `v1` tag |
| `review.yml` | pull requests | runs the action on this repository's own pull requests |

The action itself (`action.yml`) downloads the release binary that matches its
own version from the Releases page and checks it against `checksums.txt`.
Used from a branch or commit instead of a release tag, it builds from source.

## One-time Marketplace setup

Done once, before the first release:

- [ ] Repository is public.
- [ ] Accept the Marketplace developer agreement (needs two-factor auth).
      GitHub prompts for this the first time you tick the Marketplace box.
- [ ] On the first release, pick the Marketplace categories (suggested:
      *Code review*, *Continuous integration*).
- [ ] After the first Docker push, open the package settings on GitHub and set
      the `lgtm` package's visibility to **public**. New packages start private.
- [ ] If tag protection rules are on, allow GitHub Actions to update `v*`
      tags, or `publish.yml` can't move `v1`.
