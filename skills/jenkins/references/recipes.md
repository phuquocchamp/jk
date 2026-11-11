# jk recipes

All commands use `--json`. `JOB` is a job path such as `team-a/api` or a
multibranch branch such as `team-a/api/main`.

## Debug a failed pipeline

Every command here is read-only, so each one carries `--tier read`.

```sh
# 1. Pin the build number. The latest build may be green while an older one failed.
jk job get JOB --tier read --json --fields last_build,last_failed_build,last_successful_build
jk build get JOB 57 --tier read --json --fields number,result,building,causes,revision,changes

# 2. Which stages failed? NOT_EXECUTED (skipped) is left out on purpose.
jk build stages JOB 57 --tier read --json \
  --jq '[.stages[] | select(.status | IN("FAILED","UNSTABLE","ABORTED")) | {id,name,status,start_time,duration_ms}]'

# 3. Read the failed stages in start_time order, by id. Skip any with total_lines: 0
#    (a parent stage, or one that failed because of an earlier stage).
jk log tail JOB 57 --stage 479 -n 300 --tier read --json --fields text,lines,total_lines,truncated

# 4. Failed JUnit tests, if a report exists.
jk test failed JOB 57 --limit 30 --tier read --json \
  --jq '{has_report, fail_count, failed: [.failed[] | {class_name, name, error_details}]}'

# 5. Last resort: search the whole console without downloading it.
jk log search JOB 57 -e 'ERROR|Exception|exit code [1-9]|BUILD FAILURE' -C 5 --max-matches 20 --tier read --json
```

Tips:
- Console-wide searches are noisy: test names that contain "error", agent
  reconnect messages and tool warnings all match. Stage logs are more reliable.
- `log search` with `truncated: true` stopped at `--max-matches`; the error may
  be further down. Its `scanned_lines` is how far it read, not the log length.
- `log tail` without `--stage` reads the end of the whole console. The end is
  often only post-build cleanup, so prefer `--stage` or `log search`.
- Line counts are capped by the profile's `max_log_lines` (default 2000).
  `truncated: true` means lines were left out.
- If the result has `spilled_to`, the output was larger than 64KB and was
  written to that local file. Grep that file instead of asking for more.
- Compare with the last good build:
  `jk build get JOB lastSuccessful --json --fields number,revision,changes`.

## Find a flaky test

```sh
# Last few build numbers and results
jk job get JOB --json --fields last_build,last_successful_build,last_failed_build
```

Collect the failed test names of each build, one `jk` call per build (a shell
loop falls outside the `Bash(jk:*)` permission), then count the names yourself:

```sh
jk test failed JOB 50 --limit 200 --tier read --json --jq '{number, failed: [.failed[] | "\(.class_name).\(.name)"]}'
jk test failed JOB 51 --limit 200 --tier read --json --jq '{number, failed: [.failed[] | "\(.class_name).\(.name)"]}'
# ... one call per build in the range
```

A build without a test report returns `has_report: false` and an empty
`failed` list, not an error.

A test that fails in some builds but not all of them, with no related change in
`build get --fields changes`, is a flaky candidate. Check `status`:
`REGRESSION` means it passed in the previous build.

## Trigger with parameters and wait

```sh
# 1. See which parameters the job takes, with defaults and choices.
jk job get JOB --json --jq '.parameters[] | {name, type, default, choices}'

# 2. Preview the request.
jk build trigger JOB -p ENV=staging -p VERSION=1.4.2 --dry-run --json

# 3. Trigger and wait (needs policy tier write).
jk build trigger JOB -p ENV=staging -p VERSION=1.4.2 --wait --timeout 20m --json
```

- `-p` repeats; values may contain `=` and `,`.
- Without `--wait` the result has `queue_id`; follow it with
  `jk queue get ID --json` until `build_number` is set.
- With `--wait`, `timed_out: true` is not an error; the build keeps running.
  Continue with `build get JOB <number>`.
- If an error comes back with `queue_id` in `details`, the build was queued.
  Do not trigger again.
- Passing `-p` to a job without parameters is a USAGE error. A multibranch
  branch whose Jenkinsfile declares parameters only knows them after its
  first build.

## Multibranch paths

```sh
jk job list team-a/api --json --fields name,path,color,total,truncated
```

- Use the `path` field from `job list` as-is.
- Jenkins stores a branch named `feature/login` as `feature%2Flogin`, so the
  path is `team-a/api/feature%2Flogin`. `team-a/api/feature/login` is read as
  three levels and gives NOT_FOUND.
- Pull requests usually appear as `PR-123`: `team-a/api/PR-123`.

## Paste a Jenkins URL

Any job or build URL works in place of `JOB [ref]`:

```sh
jk build get    https://jenkins.example.com/job/team-a/job/api/job/main/42/ --json
jk build stages 'https://jenkins.example.com/job/team-a/job/api/job/feature%252Flogin/7/console' --json
jk job get      https://jenkins.example.com/job/team-a/job/api/ --json
```

- The build number in the URL is used when no ref is passed. An explicit ref
  wins over the URL.
- Quote URLs in the shell, especially ones that contain `%`.
- Trailing pages such as `/console`, `/testReport` or `/pipeline-graph` are
  ignored.

## Stop a build (danger tier)

```sh
jk build stop JOB 57 --json        # exit 7, plan in error.details
# show the plan to the user and wait for approval, then:
jk build stop JOB 57 --yes --json
```

Use the build number from the plan, never `last`, so you stop the build the
user approved.
