#!/bin/bash
#
# check-template-drift.sh - Detect drift between templates and generated files
#
# This script compares files in .fray/llm/ against their source templates
# in internal/db/templates/. It's meant for fray developers to ensure
# local changes get propagated back to templates.
#
# Usage:
#   ./scripts/check-template-drift.sh          # Check for drift
#   SKIP_TEMPLATE_CHECK=1 git commit ...       # Skip the check
#
# Exit codes:
#   0 - No drift detected (or check skipped)
#   1 - Drift detected - update templates or set SKIP_TEMPLATE_CHECK=1

set -e

# Allow skipping the check
if [ "${SKIP_TEMPLATE_CHECK:-}" = "1" ]; then
    exit 0
fi

# Only run in the fray repo (check for internal/db/templates/)
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || exit 0
if [ ! -d "$REPO_ROOT/internal/db/templates" ]; then
    exit 0
fi

# Template mappings: source -> destination
# Format: "template_file:generated_file"
MAPPINGS=(
    "internal/db/templates/mentions.mld:.fray/llm/routers/mentions.mld"
    "internal/db/templates/stdout-repair.mld:.fray/llm/routers/stdout-repair.mld"
    "internal/db/templates/status.mld:.fray/llm/status.mld"
)

drift_found=0
drifted_files=()

for mapping in "${MAPPINGS[@]}"; do
    template="${mapping%%:*}"
    generated="${mapping##*:}"

    template_path="$REPO_ROOT/$template"
    generated_path="$REPO_ROOT/$generated"

    # Skip if either file doesn't exist
    [ -f "$template_path" ] || continue
    [ -f "$generated_path" ] || continue

    # Check if generated file is staged (modified)
    if git diff --cached --name-only | grep -q "^${generated}$"; then
        # Compare staged version of generated file to template
        staged_content=$(git show ":$generated" 2>/dev/null) || continue
        template_content=$(cat "$template_path")

        if [ "$staged_content" != "$template_content" ]; then
            drift_found=1
            drifted_files+=("$generated (template: $template)")
        fi
    fi
done

if [ $drift_found -eq 1 ]; then
    echo "⚠️  Template drift detected!"
    echo ""
    echo "The following .fray/llm/ files differ from their templates:"
    for file in "${drifted_files[@]}"; do
        echo "  - $file"
    done
    echo ""
    echo "Options:"
    echo "  1. Update the template to match your changes"
    echo "  2. Skip this check: SKIP_TEMPLATE_CHECK=1 git commit ..."
    echo ""
    exit 1
fi

exit 0
