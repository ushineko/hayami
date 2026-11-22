#!/usr/bin/env bash
# Watch CI for a pull request, a tag or a commit, and say plainly how it ended.
#
#   tools/watch-ci.sh --pr 378
#   tools/watch-ci.sh --ref v0.21.0
#   tools/watch-ci.sh --ref 57676c6 --all --timeout 90
#
# It resolves what it is given to a commit and watches that commit's workflow
# runs for it -- a tag's or a branch's by that name, a pull request's by its
# branch, so a merge commit's runs on main do not stand in for its tag's -- by
# each job's latest attempt, until they end. A run is done only when the run
# itself says so and has jobs: a re-run lists its jobs a few seconds after it
# starts. A list of a pull request's checks is not used: read too early it
# holds only the checks registered so far, and read after a re-run it can hold
# the old attempt's result, and both read as "done".
#
# As soon as a job fails, it prints FAILED, the job, and what its log says
# failed -- the tests, panics and errors -- and exits 1, unless --all asks it to
# wait for the rest. A job's log can be read once that job has ended, while
# others still run.
#
# Exit status: 0 every job passed (or was skipped); 1 a job failed, was
# cancelled or timed out; 2 the watch timed out or could not start.
set -uo pipefail

REPO=""
PR=""
REF=""
ALL=0
TIMEOUT_MIN=60
INTERVAL=20

usage() {
    sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
    echo
    echo "options: --pr N | --ref REF   [--repo OWNER/NAME] [--all] [--timeout MINUTES]"
}

while [ $# -gt 0 ]; do
    case "$1" in
        --pr) shift; PR="${1:-}" ;;
        --ref) shift; REF="${1:-}" ;;
        --repo) shift; REPO="${1:-}" ;;
        --all) ALL=1 ;;
        --timeout) shift; TIMEOUT_MIN="${1:-60}" ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
    shift
done

if [ -z "$PR" ] && [ -z "$REF" ]; then
    usage >&2
    exit 2
fi
for tool in gh jq; do
    command -v "$tool" >/dev/null || { echo "watch-ci: $tool is not installed" >&2; exit 2; }
done
if [ -z "$REPO" ]; then
    REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null) || {
        echo "watch-ci: no --repo, and this is not a GitHub checkout" >&2
        exit 2
    }
fi

say() { printf '%s  %s\n' "$(date +%H:%M:%S)" "$*"; }

# The commit to watch, and the branch or tag its runs are named for: a PR's
# head branch, the ref given, or none for a bare commit.
BRANCH=""
if [ -n "$PR" ]; then
    read -r SHA BRANCH < <(gh pr view "$PR" -R "$REPO" --json headRefOid,headRefName -q '.headRefOid + " " + .headRefName') ||
        { echo "watch-ci: no PR $PR" >&2; exit 2; }
    WHAT="PR #$PR"
else
    SHA=$(gh api "repos/$REPO/commits/$REF" -q .sha 2>/dev/null) || { echo "watch-ci: no commit for $REF" >&2; exit 2; }
    WHAT="$REF"
    case "$SHA" in "$REF"*) ;; *) BRANCH="$REF" ;; esac
fi
say "watching $WHAT in $REPO, commit ${SHA:0:7}"

deadline=$(( $(date +%s) + TIMEOUT_MIN * 60 ))

# runs prints "id<TAB>status" for each workflow run of the commit that belongs
# to what is watched: the branch's or tag's own, where there is a name.
runs() {
    gh api "repos/$REPO/actions/runs?head_sha=$SHA&per_page=50" 2>/dev/null |
        jq -r --arg b "$BRANCH" '.workflow_runs[] | select($b == "" or .head_branch == $b) | [.id, .status] | @tsv'
}

# jobs prints each job of a run's latest attempt as "id<TAB>status<TAB>conclusion<TAB>workflow / name".
jobs() {
    gh api "repos/$REPO/actions/runs/$1/jobs?filter=latest&per_page=100" 2>/dev/null |
        jq -r '.jobs[] | [.id, .status, (.conclusion // ""), (.workflow_name + " / " + .name)] | @tsv'
}

# why prints what a failed job's log says failed.
why() {
    gh api --allow-escape-sequences "repos/$REPO/actions/jobs/$1/logs" 2>/dev/null |
        sed -e 's/\x1b\[[0-9;]*m//g' -e 's/^[0-9]\{4\}-[0-9-]*T[0-9:.]*Z //' |
        grep -E -e '--- FAIL' -e '^FAIL' -e '^panic:' -e 'fatal error' -e 'Test killed' \
            -e '^\s+Error:' -e '^\s+Messages:' -e '_test\.go:[0-9]+:' -e '##\[error\]' \
            -e 'level=error' |
        grep -v '^\s*$' | head -40 | sed 's/^/      /'
}

# Wait for the runs to exist: a push or a tag starts them a few seconds late.
RUNS=""
while [ -z "$RUNS" ]; do
    RUNS=$(runs)
    [ -n "$RUNS" ] && break
    if [ "$(date +%s)" -ge "$deadline" ]; then
        say "TIMED OUT: no workflow run started for ${SHA:0:7}"
        exit 2
    fi
    sleep "$INTERVAL"
done

reported=""
last=""
while :; do
    # A run that starts later (a re-run, a second workflow) is picked up too.
    RUNS=$(runs)
    all=""
    unsettled=0
    while IFS=$'\t' read -r run run_status; do
        [ -z "$run" ] && continue
        listed=$(jobs "$run")
        # A run that has not ended, or has no jobs listed yet, is not done
        # whatever its listed jobs say.
        if [ "$run_status" != "completed" ] || [ -z "$listed" ]; then
            unsettled=1
        fi
        all+="$listed"$'\n'
    done <<<"$RUNS"
    all=$(printf '%s' "$all" | grep -v '^$')

    total=$(printf '%s\n' "$all" | grep -c . || true)
    done_jobs=$(printf '%s\n' "$all" | awk -F'\t' '$2=="completed"' | grep -c . || true)
    running=$(printf '%s\n' "$all" | awk -F'\t' '$2!="completed" {print $4}' | paste -sd ', ' -)
    failed=$(printf '%s\n' "$all" | awk -F'\t' '$2=="completed" && ($3=="failure" || $3=="cancelled" || $3=="timed_out" || $3=="startup_failure")')

    # Each failure, once, with its reasons.
    if [ -n "$failed" ]; then
        while IFS=$'\t' read -r id _ conclusion name; do
            case " $reported " in *" $id "*) continue ;; esac
            reported+=" $id"
            say "FAILED: $name ($conclusion)"
            reasons=$(why "$id")
            if [ -n "$reasons" ]; then
                printf '%s\n' "$reasons"
            else
                echo "      (no test or error lines in its log: https://github.com/$REPO/actions/runs)"
            fi
        done <<<"$failed"
        if [ "$ALL" = 0 ]; then
            [ -n "$running" ] && say "still running, not waited for: $running"
            exit 1
        fi
    fi

    status="$done_jobs/$total jobs done${running:+; running: $running}"
    [ "$unsettled" = 1 ] && [ -z "$running" ] && status+="; a run has not listed its jobs yet"
    if [ "$status" != "$last" ]; then
        say "$status"
        last="$status"
    fi

    if [ "$unsettled" = 0 ] && [ "$total" -gt 0 ] && [ "$done_jobs" -eq "$total" ]; then
        if [ -n "$reported" ]; then
            say "FAILED: $(wc -w <<<"$reported") job(s) failed"
            exit 1
        fi
        printf '%s\n' "$all" | awk -F'\t' '{printf "      %-10s %s\n", ($3==""?"-":$3), $4}'
        say "PASSED: every job of ${SHA:0:7} passed or was skipped"
        exit 0
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
        say "TIMED OUT after ${TIMEOUT_MIN}m; still running: $running"
        exit 2
    fi
    sleep "$INTERVAL"
done
