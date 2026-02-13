#!/bin/bash
# End-to-end validation of genainormalizer processor
# Runs 8 real agent scenarios across 4 frameworks × 2 instrumentation libraries
#
# Prerequisites:
#   - AWS credentials configured (agents call Bedrock)
#   - uv installed (https://astral.sh/uv)
#   - Custom collector built: builder --config builder-config.yaml
#
# Usage:
#   ./test-agents/run-all.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
LOG_FILE="/tmp/otelcol-genai-e2e.log"

# Start collector
pkill -f otelcol-genai 2>/dev/null || true
sleep 1
"$REPO_DIR/dist/otelcol-genai" --config "$REPO_DIR/collector-config.yaml" > "$LOG_FILE" 2>&1 &
COLLECTOR_PID=$!
sleep 3

if ! kill -0 $COLLECTOR_PID 2>/dev/null; then
  echo "ERROR: Collector failed to start. Check $LOG_FILE"
  exit 1
fi
echo "Collector started (PID: $COLLECTOR_PID)"

AGENTS=(
  "strands-openinference"
  "strands-openllmetry"
  "langchain-openinference"
  "langchain-openllmetry"
  "langgraph-openinference"
  "langgraph-openllmetry"
  "crewai-openinference"
  "crewai-openllmetry"
  "pydanticai-openinference"
)

PASSED=0
FAILED=0

for agent in "${AGENTS[@]}"; do
  echo ""
  echo "=== $agent ==="
  cd "$SCRIPT_DIR/$agent"
  if timeout 90 uv run agent.py 2>&1 | tail -3; then
    PASSED=$((PASSED + 1))
  else
    echo "FAILED"
    FAILED=$((FAILED + 1))
  fi
  sleep 2
done

# Wait for final flush
sleep 3

echo ""
echo "=========================================="
echo "RESULTS: $PASSED passed, $FAILED failed"
echo "=========================================="
echo ""

# Analyze output
echo "=== Spans processed ==="
grep -c "Span #0" "$LOG_FILE" || echo "0"

echo ""
echo "=== gen_ai.operation.name values ==="
grep "gen_ai.operation.name" "$LOG_FILE" | grep -o "Str([^)]*)" | sort | uniq -c | sort -rn

echo ""
echo "=== gen_ai.request.model values ==="
grep "gen_ai.request.model" "$LOG_FILE" | grep -o "Str([^)]*)" | sort | uniq -c | sort -rn

echo ""
echo "=== gen_ai.usage.input_tokens count ==="
grep -c "gen_ai.usage.input_tokens" "$LOG_FILE" || echo "0"

echo ""
echo "=== gen_ai.usage.output_tokens count ==="
grep -c "gen_ai.usage.output_tokens" "$LOG_FILE" || echo "0"

echo ""
echo "=== Services ==="
grep "service.name:" "$LOG_FILE" | grep -o "Str([^)]*)" | sort | uniq -c | sort -rn

# Cleanup
kill $COLLECTOR_PID 2>/dev/null
echo ""
echo "Full log: $LOG_FILE"
