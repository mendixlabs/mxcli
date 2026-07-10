#!/bin/bash
# Generate Go parser from MDL grammar

set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}/parser"

echo "Generating Go parser from MDL grammar..."
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

antlr4 -Dlanguage=Go -package parser -lib domains -o "${OUTPUT_DIR}" MDLLexer.g4 MDLParser.g4

echo "Generated files:"
ls -la "${OUTPUT_DIR}"
