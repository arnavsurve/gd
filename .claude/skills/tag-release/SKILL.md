---
name: tag-release
description: Tag and push a new release. Goreleaser GitHub Action handles the rest.
disable-model-invocation: true
argument-hint: "[version, e.g. v0.2.0]"
allowed-tools: Bash(git:*), Bash(goreleaser:*), Bash(gh:*)
---

# Release

Release version $ARGUMENTS.

1. If no version argument is provided, check the latest tag with `git tag --sort=-v:refname | head -1` and suggest the next patch bump.
2. Confirm the working tree is clean (`git status --porcelain`). If not, stop and tell the user.
3. Run `goreleaser check` to validate the config.
4. Run `goreleaser release --snapshot --skip=publish --clean` to do a dry run. If it fails, stop.
5. Create the tag: `git tag $ARGUMENTS`
6. Push the tag: `git push origin $ARGUMENTS`
7. Print a link to the GitHub Actions run: `gh run list --workflow=release.yml --limit=1`
8. Tell the user goreleaser will build and attach binaries automatically.
