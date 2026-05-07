#!/bin/bash
# Runs all failing RolloutPlugin e2e tests IN PARALLEL and collects
# StatefulSet + RolloutPlugin snapshots every 2 seconds per test.
# Each test has a unique resource name so they can safely run concurrently.
#
# Results are saved to snapshot-results/<testname>/ with:
#   - test-output.log  (test binary output)
#   - snapshots.log    (kubectl snapshots during the run)
#   - result.txt       (PASS or FAIL)

set -uo pipefail

export NAMESPACE="default"
export SNAPSHOT_DIR="$(pwd)/snapshot-results-one"
export INTERVAL=2
export MAX_PARALLEL="${MAX_PARALLEL:-10}"
export TEST_BINARY="$(pwd)/dist/e2e-rolloutplugin.test"

# Pre-install dev tools once before parallel execution
echo "Installing dev tools..."
make install-devtools-local
echo ""

# Pre-compile the test binary once to avoid parallel build races
echo "Pre-compiling test binary..."
go test -c -tags e2e -o "$TEST_BINARY" ./test/e2e
echo "Test binary: $TEST_BINARY"
echo ""

# Test name -> resource name mapping (all unique)
declare -A TEST_RESOURCE_MAP
TEST_RESOURCE_MAP[TestRolloutPluginAbort]=rp-abort
TEST_RESOURCE_MAP[TestRolloutPluginAbortBeforePause]=rp-abort-early
TEST_RESOURCE_MAP[TestRolloutPluginAbortEvent]=rp-abort-event
TEST_RESOURCE_MAP[TestRolloutPluginAbortIdempotent]=rp-abort-idem
TEST_RESOURCE_MAP[TestRolloutPluginAnalysisRunOwnership]=rp-ar-ownership
TEST_RESOURCE_MAP[TestRolloutPluginBackgroundAnalysisFail]=rp-bg-analysis-fail
TEST_RESOURCE_MAP[TestRolloutPluginBackgroundAnalysisSuccess]=rp-bg-analysis-ok
TEST_RESOURCE_MAP[TestRolloutPluginBasicCanary]=rp-basic-canary
TEST_RESOURCE_MAP[TestRolloutPluginCanaryMultipleSteps]=rp-multi-steps
TEST_RESOURCE_MAP[TestRolloutPluginCanaryNoSteps]=rp-no-steps
TEST_RESOURCE_MAP[TestRolloutPluginConditions]=rp-conditions
TEST_RESOURCE_MAP[TestRolloutPluginEvents]=rp-events
TEST_RESOURCE_MAP[TestRolloutPluginInitialDeploy]=rp-initial
TEST_RESOURCE_MAP[TestRolloutPluginInlineAnalysisFail]=rp-inline-fail
TEST_RESOURCE_MAP[TestRolloutPluginInlineAnalysisSuccess]=rp-inline-ok
TEST_RESOURCE_MAP[TestRolloutPluginInvalidSpecFix]=rp-invalid-fix
TEST_RESOURCE_MAP[TestRolloutPluginInvalidSpecMissingStrategy]=rp-invalid-strategy
TEST_RESOURCE_MAP[TestRolloutPluginInvalidSpecPluginNotFound]=rp-invalid-plugin
TEST_RESOURCE_MAP[TestRolloutPluginManualPause]=rp-manual-pause
TEST_RESOURCE_MAP[TestRolloutPluginMultipleRestarts]=rp-multi-restart
TEST_RESOURCE_MAP[TestRolloutPluginNewRevisionClearsAbort]=rp-new-rev-abort
TEST_RESOURCE_MAP[TestRolloutPluginNewRevisionMidRollout]=rp-new-rev
TEST_RESOURCE_MAP[TestRolloutPluginObservedGeneration]=rp-obs-gen
TEST_RESOURCE_MAP[TestRolloutPluginPauseResumeDuringRollout]=rp-pause-resume
TEST_RESOURCE_MAP[TestRolloutPluginProgressDeadlineAbort]=rp-deadline-abort
TEST_RESOURCE_MAP[TestRolloutPluginProgressDeadlineNoAbort]=rp-deadline-no-abort
TEST_RESOURCE_MAP[TestRolloutPluginPromoteFull]=rp-promote-full
TEST_RESOURCE_MAP[TestRolloutPluginPromoteFullFromFirstStep]=rp-promote-full-first
TEST_RESOURCE_MAP[TestRolloutPluginRestartAfterAbort]=rp-restart
TEST_RESOURCE_MAP[TestRolloutPluginRestartRejectedWithoutAbort]=rp-restart-reject
TEST_RESOURCE_MAP[TestRolloutPluginStatefulSetPartition]=rp-partition
TEST_RESOURCE_MAP[TestRolloutPluginStatusFields]=rp-status-fields
TEST_RESOURCE_MAP[TestRolloutPluginStepPauseAndPromote]=rp-step-pause
TEST_RESOURCE_MAP[TestRolloutPluginTimedPause]=rp-timed-pause
export TEST_RESOURCE_MAP

# When running tests in parallel, each test binary that finishes calls TearDownSuite
# which (without this flag) deletes ALL e2e-labeled resources in the namespace — including
# resources owned by still-running parallel tests. E2E_DEBUG=true skips TearDownSuite
# and AfterTest cleanup, while BeforeTest still cleans up only the current test's own
# resources before it starts. A final cleanup is done at the bottom of this script.
export E2E_DEBUG=true

mkdir -p "$SNAPSHOT_DIR"

# Collect snapshots for a specific resource name
collect_snapshots() {
  local outfile="$1"
  local res_name="$2"
  local ns="$3"
  while true; do
    {
      echo "=== $(date -Iseconds) ==="
      echo "--- StatefulSet: $res_name ---"
      kubectl get statefulset "$res_name" -n "$ns" -o wide 2>/dev/null || echo "(not found)"
      echo ""
      echo "--- StatefulSet updateStrategy ---"
      kubectl get statefulset "$res_name" -n "$ns" -o jsonpath='{.spec.updateStrategy}' 2>/dev/null || true
      echo ""
      echo "--- StatefulSet status ---"
      kubectl get statefulset "$res_name" -n "$ns" -o jsonpath='{.status}' 2>/dev/null || true
      echo ""
      echo "--- RolloutPlugin: $res_name ---"
      kubectl get rolloutplugin "$res_name" -n "$ns" -o wide 2>/dev/null || echo "(not found)"
      echo ""
      echo "--- RolloutPlugin status ---"
      kubectl get rolloutplugin "$res_name" -n "$ns" -o jsonpath='{.status}' 2>/dev/null || true
      echo ""
      echo ""
    } >> "$outfile"
    sleep "$INTERVAL"
  done
}

# Run a single test with snapshot collection
run_one() {
  local test_name="$1"
  local res_name="$2"
  local test_dir="$SNAPSHOT_DIR/$test_name"
  mkdir -p "$test_dir"

  echo "[START] $test_name (resource: $res_name)"

  # Start snapshot collector in background
  collect_snapshots "$test_dir/snapshots.log" "$res_name" "$NAMESPACE" &
  local spid=$!

  # Run the test using the pre-compiled binary.
  # Anchor the -testify.m pattern with ^ and $ to prevent regex substring matches
  # (e.g. "TestRolloutPluginAbort" would otherwise also run AbortBeforePause, AbortIdempotent, etc.)
  local rc=0
  "$TEST_BINARY" -test.timeout 600s -test.count 1 -test.v -test.short -test.run "TestRolloutPluginSuite" -testify.m "^${test_name}$" > "$test_dir/test-output.log" 2>&1 || rc=$?

  # Stop snapshot collector
  kill "$spid" 2>/dev/null || true
  wait "$spid" 2>/dev/null || true

  if [[ $rc -eq 0 ]]; then
    echo "PASS" > "$test_dir/result.txt"
    echo "[PASS] $test_name"
  else
    echo "FAIL" > "$test_dir/result.txt"
    echo "[FAIL] $test_name"
  fi
}

# If a previous summary exists, only re-run FAIL tests; otherwise run all.
PREV_SUMMARY="$SNAPSHOT_DIR/summary.txt"
if [[ -f "$PREV_SUMMARY" ]]; then
  FAILED_TESTS=$(grep '^FAIL' "$PREV_SUMMARY" | awk '{print $2}')
  if [[ -z "$FAILED_TESTS" ]]; then
    echo "No failed tests in previous summary. Nothing to re-run."
    exit 0
  fi
  ALL_TESTS="$FAILED_TESTS"
  TOTAL=$(echo "$ALL_TESTS" | wc -w)
  echo "Re-running $TOTAL previously-failed tests only."
else
  ALL_TESTS="${!TEST_RESOURCE_MAP[*]}"
  TOTAL=${#TEST_RESOURCE_MAP[@]}
fi

echo "Starting parallel test execution (max $MAX_PARALLEL concurrent)..."
echo "Results will be saved to: $SNAPSHOT_DIR"
echo "Tests: $TOTAL"
echo ""

# Clean only the dirs for tests we're about to run
for TEST in $ALL_TESTS; do
  rm -rf "$SNAPSHOT_DIR/$TEST"
done

pids=()
for TEST in $ALL_TESTS; do
  RES="${TEST_RESOURCE_MAP[$TEST]}"
  run_one "$TEST" "$RES" &
  pids+=($!)

  # Throttle: if we hit MAX_PARALLEL, wait for one to finish
  if [[ ${#pids[@]} -ge $MAX_PARALLEL ]]; then
    wait -n "${pids[@]}" 2>/dev/null || true
    # Rebuild pids array removing finished ones
    new_pids=()
    for p in "${pids[@]}"; do
      if kill -0 "$p" 2>/dev/null; then
        new_pids+=("$p")
      fi
    done
    pids=("${new_pids[@]}")
  fi
done

# Wait for all remaining jobs
wait

echo ""
echo "========================================"
echo "SUMMARY"
echo "========================================"

for TEST in $(echo "${!TEST_RESOURCE_MAP[*]}" | tr ' ' '\n' | sort); do
  if [[ -f "$SNAPSHOT_DIR/$TEST/result.txt" ]]; then
    RESULT=$(cat "$SNAPSHOT_DIR/$TEST/result.txt")
  else
    RESULT="NO_RESULT"
  fi
  echo "$RESULT  $TEST"
done | tee "$SNAPSHOT_DIR/summary.txt"

echo ""
echo "Full results saved to: $SNAPSHOT_DIR"
echo "(Skipping resource cleanup — resources left in cluster for debugging.)"
