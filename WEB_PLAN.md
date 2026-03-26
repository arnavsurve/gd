# Semantic PR Review — Web App

## Overview

A hosted web app for GitHub PR review with entity-level semantic diffs. Lives in `web/` inside the gd repo. Uses `sem` (tree-sitter semantic diffing) as the engine and `@pierre/diffs` as the rendering layer. Deployed as a Next.js app.

The core idea: GitHub's diff view shows you lines that changed. This shows you *what* changed — functions, classes, types — grouped by structural unit, with move/rename detection and cosmetic-change filtering.

## Stack

| Layer | Tool | Why |
|-------|------|-----|
| Semantic engine | `sem` CLI (Rust, tree-sitter) | 21 languages, rename/move detection, structural hashing, JSON output. No point rebuilding this. |
| Diff rendering | `@pierre/diffs` (TypeScript, Shiki) | Split/unified layouts, inline comments, annotations, virtual scrolling. Production-grade. |
| Framework | Next.js (App Router) | API routes for backend, React for frontend, easy deployment. |
| Auth | GitHub OAuth (via NextAuth) | Access private repos, post review comments back to GitHub. |
| Hosting | Vercel | Zero-config Next.js deployment, edge functions for API routes. |

## Architecture

```
Browser                       Vercel
  │                             │
  │  /owner/repo/pull/123       │
  │ ──────────────────────────> │
  │                             │
  │                        ┌────┴────┐
  │                        │ Next.js │
  │                        │ API     │
  │                        └────┬────┘
  │                             │
  │                     ┌───────┼───────┐
  │                     │       │       │
  │                     v       v       v
  │                  GitHub   sem     cache
  │                  API      CLI     (KV)
  │                     │       │       │
  │                     └───────┼───────┘
  │                             │
  │  semantic diff JSON         │
  │ <────────────────────────── │
  │                             │
  │  (rendered client-side      │
  │   via @pierre/diffs)        │
```

## Data Flow

### 1. User navigates to a PR

Route: `/[owner]/[repo]/pull/[number]`

### 2. API fetches PR data via GraphQL

```
GET /api/diff?owner=X&repo=Y&pr=123
```

A single GraphQL query fetches PR metadata, file list, and file contents in one round trip:

```graphql
query PRDiff($owner: String!, $repo: String!, $pr: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $pr) {
      title
      state
      baseRefOid
      headRefOid
      baseRef { name }
      headRef { name }
      files(first: 100) {
        nodes {
          path
          additions
          deletions
          changeType          # ADDED, CHANGED, DELETED, RENAMED, COPIED
          previousFilename    # for renames
        }
        pageInfo { hasNextPage endCursor }
      }
      reviews(last: 50) {
        nodes {
          state
          body
          author { login }
          comments(first: 100) {
            nodes {
              path
              line
              body
              author { login }
              createdAt
            }
          }
        }
      }
      comments(first: 50) {
        nodes { body author { login } createdAt }
      }
    }
  }
}
```

Then fetch file contents in a second batched query using the base/head OIDs:

```graphql
query FileContents($owner: String!, $repo: String!, $expressions: [String!]!) {
  repository(owner: $owner, name: $repo) {
    # dynamically aliased per file
    base_file0: object(expression: $expressions[0]) { ... on Blob { text } }
    head_file0: object(expression: $expressions[1]) { ... on Blob { text } }
    base_file1: object(expression: $expressions[2]) { ... on Blob { text } }
    head_file1: object(expression: $expressions[3]) { ... on Blob { text } }
    # ...
  }
}
```

Each expression is `"{ref}:{path}"`, e.g. `"abc1234:src/auth.ts"`. This fetches all old+new file contents in a single request — no N+1 REST calls.

For PRs with >100 files, paginate the `files` connection using `endCursor`.

Backend pipeline:
1. Run the PR metadata query → get file list + base/head SHAs + existing review comments
2. Build expressions list, run the file contents query → get all old/new source in one call
3. Build the `sem` stdin payload from the results:
   ```json
   [
     {
       "filePath": "src/auth.ts",
       "status": "modified",
       "beforeContent": "...",
       "afterContent": "..."
     }
   ]
   ```
4. Pipe through `sem diff --stdin --format json`
5. Return structured response to frontend (sem changes + review comments)

### 3. Frontend renders

For each file, group the `sem` changes by file path. For each entity change:
- Render a structural header (entity type, name, change kind)
- Pass `beforeContent` / `afterContent` to `@pierre/diffs` `<FileDiff>` component
- Cosmetic-only changes (where `structuralChange === false`) get a dimmed treatment
- Existing review comments rendered as `@pierre/diffs` annotations on the correct lines

## sem JSON Schema

Input (stdin mode):
```json
[
  {
    "filePath": "src/main.ts",
    "status": "modified",
    "beforeContent": "function foo() { return 1; }",
    "afterContent": "function foo() { return 2; }"
  }
]
```

Output:
```json
{
  "summary": {
    "fileCount": 1,
    "added": 0,
    "modified": 1,
    "deleted": 0,
    "total": 1
  },
  "changes": [
    {
      "entityId": "src/main.ts::function::foo",
      "changeType": "modified",
      "entityType": "function",
      "entityName": "foo",
      "filePath": "src/main.ts",
      "beforeContent": "function foo() { return 1; }",
      "afterContent": "function foo() { return 2; }",
      "structuralChange": true
    }
  ]
}
```

Change types: `added`, `modified`, `deleted`, `moved`, `renamed`

## Directory Structure

```
web/
├── app/
│   ├── layout.tsx
│   ├── page.tsx                          # landing page, GitHub connect
│   ├── auth/
│   │   └── [...nextauth]/route.ts        # GitHub OAuth via NextAuth
│   ├── [owner]/[repo]/
│   │   ├── page.tsx                      # repo overview (recent PRs)
│   │   └── pull/[number]/
│   │       └── page.tsx                  # PR review view
│   └── api/
│       └── diff/route.ts                 # sem pipeline endpoint
├── components/
│   ├── semantic-diff-view.tsx            # groups entity changes, renders headers
│   ├── entity-diff.tsx                   # single entity: header + @pierre/diffs FileDiff
│   ├── file-tree.tsx                     # sidebar file list with change counts
│   ├── change-summary.tsx                # "3 modified · 1 added · 1 moved"
│   ├── review-toolbar.tsx                # approve/request changes/comment
│   └── comment-thread.tsx                # inline comment UI
├── lib/
│   ├── github.ts                         # GraphQL client (PR metadata, file contents, review mutations)
│   ├── queries.ts                        # GraphQL query/mutation strings
│   ├── sem.ts                            # spawn sem CLI, parse JSON output
│   └── types.ts                          # shared types for sem output + GraphQL responses
├── package.json
├── next.config.ts
└── tsconfig.json
```

## Key Components

### `semantic-diff-view.tsx`

The main review component. Takes sem's JSON output and renders it as a list of entity-level diffs.

```tsx
interface SemanticDiffViewProps {
  changes: SemChange[]
  summary: SemSummary
}
```

For each change:
- Render a collapsible structural header: `▸ function processOrder (modified)`
- Inside: `<FileDiff>` from `@pierre/diffs` with the entity's before/after content
- Cosmetic-only changes (`structuralChange: false`) collapsed by default with a "formatting only" badge
- Moved/renamed entities show old → new location

### `entity-diff.tsx`

Wraps `@pierre/diffs` `<FileDiff>` for a single entity.

```tsx
import { FileDiff, Virtualizer } from '@pierre/diffs/react'

<Virtualizer>
  <FileDiff
    oldFile={{ path: change.filePath, content: change.beforeContent }}
    newFile={{ path: change.filePath, content: change.afterContent }}
    annotations={comments}
  />
</Virtualizer>
```

### `file-tree.tsx`

Sidebar showing changed files grouped by directory, with per-file entity change counts:

```
src/
  auth/
    login.ts        3 entities
    session.ts      1 entity
  config/
    database.yml    1 entity
```

Click a file to scroll to its section.

## API Route: `/api/diff`

```ts
// app/api/diff/route.ts

export async function GET(req: Request) {
  const { owner, repo, pr } = parseParams(req)
  const token = await getGitHubToken(req)

  // 1. GraphQL: PR metadata + file list + review comments
  const prData = await graphql<PRQuery>(PR_DIFF_QUERY, {
    owner, repo, pr,
    headers: { authorization: `bearer ${token}` },
  })

  const { baseRefOid, headRefOid, files, reviews } = prData.repository.pullRequest

  // 2. GraphQL: batch fetch all file contents in one request
  // Build dynamic query with aliased object() lookups
  const expressions = files.nodes.flatMap((f) => {
    const base = f.changeType !== 'ADDED' ? `${baseRefOid}:${f.previousFilename ?? f.path}` : null
    const head = f.changeType !== 'DELETED' ? `${headRefOid}:${f.path}` : null
    return [base, head]
  })

  const contents = await fetchFileContents(owner, repo, expressions, token)

  // 3. Build sem input from GraphQL results
  const fileChanges = files.nodes.map((f, i) => ({
    filePath: f.path,
    oldFilePath: f.previousFilename ?? undefined,
    status: mapChangeType(f.changeType),
    beforeContent: contents[i * 2] ?? undefined,
    afterContent: contents[i * 2 + 1] ?? undefined,
  }))

  // 4. Run sem
  const result = await runSem(fileChanges)

  // 5. Attach review comments for frontend annotation rendering
  return Response.json({ ...result, reviews: reviews.nodes })
}
```

### GraphQL Client: `lib/github.ts`

```ts
import { graphql } from '@octokit/graphql'

// Batch file contents via dynamically aliased object() lookups
// GitHub GraphQL supports up to ~500KB response size and 100 nodes per connection
export async function fetchFileContents(
  owner: string,
  repo: string,
  expressions: (string | null)[],
  token: string,
): Promise<(string | null)[]> {
  // Build query: each expression becomes an aliased field
  const fields = expressions
    .map((expr, i) =>
      expr ? `f${i}: object(expression: "${expr}") { ... on Blob { text } }` : ''
    )
    .filter(Boolean)
    .join('\n    ')

  const query = `query {
    repository(owner: "${owner}", name: "${repo}") {
      ${fields}
    }
  }`

  const result = await graphql<Record<string, { text: string } | null>>(query, {
    headers: { authorization: `bearer ${token}` },
  })

  return expressions.map((expr, i) =>
    expr ? result.repository[`f${i}`]?.text ?? null : null
  )
}
```

This approach means **2 GraphQL requests total** per PR regardless of file count (1 for metadata, 1 for all file contents), vs. 2N+1 REST calls with the REST API.

## Review Flow

### Reading a PR
1. Navigate to `/owner/repo/pull/123`
2. See file tree (left) + semantic diff (right)
3. Entity changes grouped by file, each with structural header
4. Toggle between semantic view and raw diff view
5. Expand/collapse individual entities or entire files

### Commenting
- Click a line in `@pierre/diffs` to open comment input
- Existing GitHub comments fetched in the initial PR query and rendered as annotations
- Comments appear both here and on GitHub — no lock-in

### Submitting a review
- Toolbar at top: Approve / Request Changes / Comment
- Uses `addPullRequestReview` GraphQL mutation with inline comments:
  ```graphql
  mutation SubmitReview($input: AddPullRequestReviewInput!) {
    addPullRequestReview(input: $input) {
      pullRequestReview { id state }
    }
  }
  ```
  Input includes `event` (APPROVE, REQUEST_CHANGES, COMMENT), `body`, and `threads` (each with `path`, `line`, `body`) — all pending comments submitted as a single atomic review

## Deployment

### sem binary on Vercel

Vercel functions run on Lambda (Amazon Linux). Options:
1. **Prebuilt binary**: Include a Linux x86_64 `sem` binary in the repo, reference from API route. Simple but adds ~15MB to the repo.
2. **Docker deployment**: Use Vercel's Docker runtime or deploy to Fly.io/Railway instead. More flexibility.
3. **WASM**: sem uses native tree-sitter (not WASM), so this isn't an option today.

Recommendation: start with option 1 (prebuilt binary in `web/bin/`). Switch to Docker on Fly.io if Vercel's function limits become a problem (10s timeout on hobby, 60s on pro).

### Caching

Cache sem results keyed by `(owner, repo, pr, headSha)`. When the head SHA changes (new push), invalidate. Use Vercel KV or just in-memory for v1.

## Phases

### Phase 1: Read-only semantic diff viewer
- GitHub OAuth
- Fetch PR files, run sem, render with @pierre/diffs
- File tree sidebar
- Structural headers and change summary
- Deploy to Vercel

### Phase 2: Review workflow
- Inline commenting (post to GitHub)
- Review submission (approve/request changes)
- Fetch and display existing GitHub comments
- Keyboard navigation between entities

### Phase 3: Polish
- Cosmetic-change filtering (hide/dim formatting-only changes)
- Moved/renamed entity visualization
- Dark/light theme toggle (reuse gd's theme palette)
- URL sharing (deep links to specific entities)
- Responsive layout for different screen sizes

## Open Questions

- **GraphQL response size**: GitHub caps GraphQL responses at ~500KB. PRs with many large files may need to be split across multiple file-content queries. Paginate the file list and batch contents in chunks of ~30 files.
- **Large files**: sem + @pierre/diffs both need full file content in memory. For very large files (>1MB) or binary files, `object()` returns null — fall back to raw diff view.
- **Private repos**: Requires GitHub OAuth with `repo` scope. Users need to trust the app with read access to their code. File contents are processed server-side and not stored beyond the cache TTL.
- **sem installation on CI/serverless**: The sem binary needs to be available in the deployment environment. This is the main operational constraint.
- **GraphQL rate limits**: GraphQL uses a point system (5000 points/hr) rather than per-request limits. Each query costs ~1 point per 100 nodes. A typical PR review is 2-3 points total — effectively unlimited for normal use.
