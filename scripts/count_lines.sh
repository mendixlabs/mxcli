#!/bin/bash
# Count lines of Go source code in the project
# Identifies large files that may need refactoring

echo "=== Go Source Code Line Counts ==="
echo ""

# Find all .go files, excluding generated code and vendor
find . -name "*.go" \
    -not -path "./vendor/*" \
    -not -path "./.git/*" \
    -not -path "./generated/*" \
    -not -path "./libs/*" \
    -not -path "./reference/*" \
    -not -path "*/parser/*.go" \
    -type f \
    -exec wc -l {} \; | sort -rn | head -30

echo ""
echo "=== Summary ==="

# Total lines
total=$(find . -name "*.go" \
    -not -path "./vendor/*" \
    -not -path "./.git/*" \
    -not -path "./generated/*" \
    -not -path "./libs/*" \
    -not -path "./reference/*" \
    -not -path "*/parser/*.go" \
    -type f \
    -exec cat {} \; | wc -l)

echo "Total Go source lines (excluding generated/vendor): $total"

# Count files
file_count=$(find . -name "*.go" \
    -not -path "./vendor/*" \
    -not -path "./.git/*" \
    -not -path "./generated/*" \
    -not -path "./libs/*" \
    -not -path "./reference/*" \
    -not -path "*/parser/*.go" \
    -type f | wc -l)

echo "Total Go source files: $file_count"

echo ""
echo "=== Files over 500 lines (candidates for refactoring) ==="
find . -name "*.go" \
    -not -path "./vendor/*" \
    -not -path "./.git/*" \
    -not -path "./generated/*" \
    -not -path "./libs/*" \
    -not -path "./reference/*" \
    -not -path "*/parser/*.go" \
    -type f \
    -exec wc -l {} \; | awk '$1 > 500 {print}' | sort -rn

echo ""
echo "=== Files over 1000 lines (should consider splitting) ==="
find . -name "*.go" \
    -not -path "./vendor/*" \
    -not -path "./.git/*" \
    -not -path "./generated/*" \
    -not -path "./libs/*" \
    -not -path "./reference/*" \
    -not -path "*/parser/*.go" \
    -type f \
    -exec wc -l {} \; | awk '$1 > 1000 {print}' | sort -rn
