# Output and errors

## Output

- Stdout carries only data. Output is JSON when `--json` or `--jq` is set, or
  when stdout is not a terminal.
- `--fields a,b` keeps only these keys (per item for lists).
- `--jq EXPR` filters the JSON result with jq
  ([gojq](https://github.com/itchyny/gojq)).
- Durations are milliseconds (`*_ms`) and timestamps RFC3339 in JSON; human
  output shows them readably.
- Lists default to `--limit 50`.

```sh
jk build get team-a/api/main lastFailed --json --fields number,result,causes
jk job list team-a --jq '.jobs[] | select(.color == "red") | .path'
```

### Logs

- Log output is capped by `max_log_lines`; `truncated: true` says lines were
  left out.
- When a log result is larger than 64KB, it is written to a file under the
  system temp dir (`$TMPDIR/jk/`, or a private `$TMPDIR/jk-*` dir when that one
  is not owned by you with mode 0700), and `spilled_to` holds its path.

### Tests and triggers

- `test failed` returns `has_report: false` when the build has no test report.
- `build trigger --wait` returns `timed_out: true` (not an error) when the
  timeout passes; the build keeps running.
- If an error happens after the build was queued, the error's `details`
  contain the `queue_id`: do not trigger again.

## Errors

Errors go to stderr. In JSON mode they look like this (`details` is present when there is a plan or partial result):

```json
{"error":{"code":"NOT_FOUND","message":"...","hint":"...","retryable":false}}
```

Read the `hint` before retrying, and retry only when `retryable` is true.

## Exit codes

| exit | code | meaning |
|---|---|---|
| 0 | | success |
| 1 | `UPSTREAM` | Jenkins error or timeout; `retryable` is true for 5xx and timeouts, except for `build trigger`/`build replay` requests that may already have reached Jenkins (check before re-running) |
| 1 | `INTERNAL` | bug in jk |
| 2 | `USAGE` | bad command, flag, argument or config |
| 3 | `NOT_FOUND` | job, build or queue item does not exist |
| 4 | `POLICY_DENIED` | blocked by the profile's tier or allow/deny globs |
| 4 | `FORBIDDEN` | Jenkins returned 403 |
| 5 | `CONFLICT` | Jenkins returned 409 |
| 6 | `AUTH_REQUIRED` | no token, or Jenkins returned 401 |
| 7 | `NEEDS_CONFIRMATION` | danger operation without `--yes` / `confirm` |
