#!/usr/bin/env bash
# Run every Tinoc sample end-to-end with the local compiler.
#
#   ./samples/build.sh           build the compiler, then run all samples
#   ./samples/build.sh check     only semantic-check each sample (no C build)
#   ./samples/build.sh <file>    run a specific sample (relative to samples/)
#
# Samples are single files; `samples/modules/` is a multi-file module
# demo with a main.tnc entry point.

set -euo pipefail
cd "$(dirname "$0")/.."   # project root

MODE="${1:-run}"

# The compiler binary: reuse build/tinoc if present, else go run.
if [ -x build/tinoc ]; then
    TINOC=(./build/tinoc)
else
    TINOC=(go run .)
fi

run_one() {
    echo "--- ${1}"
    "${TINOC[@]}" check "$1" >/dev/null 2>&1 || {
        echo "FAIL (check): $1"
        "${TINOC[@]}" check "$1" 2>&1 | tail -5
        return 1
    }
    if [ "$MODE" != "check" ] && [ "$1" != "samples/01_vars.tnc" ]; then
        # 01_vars is intentionally a declarations-only sample (no main).
        "${TINOC[@]}" run "$1" >/dev/null 2>&1 || {
            echo "FAIL (run): $1"
            "${TINOC[@]}" run "$1" 2>&1 | tail -8
            return 1
        }
    fi
    echo "ok   $1"
}

failures=0
for f in samples/0[0-9]*.tnc samples/[12][0-9]*.tnc; do
    run_one "$f" || failures=$((failures + 1))
done
run_one samples/modules/main.tnc || failures=$((failures + 1))

echo
if [ "$failures" -eq 0 ]; then
    echo "All samples passed (${MODE})."
else
    echo "${failures} sample(s) failed."
    exit 1
fi
