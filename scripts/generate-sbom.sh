#!/usr/bin/env bash
# Generate CycloneDX SBOM for Go and TypeScript dependencies.
#
# Produces sbom.cdx.json with components from:
#   1. Go modules (go.mod)
#   2. TypeScript/npm (vscode-mdl/package.json)
#
# Requires: bun, cyclonedx-gomod (go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUTPUT="${ROOT_DIR}/sbom.cdx.json"
TMP_GO="${ROOT_DIR}/.sbom-go.json"
TMP_TS="${ROOT_DIR}/.sbom-ts.json"

cleanup() {
    rm -f "$TMP_GO" "$TMP_TS"
}
trap cleanup EXIT

echo "Generating Go SBOM..."
cyclonedx-gomod mod -json -licenses -output "$TMP_GO" "$ROOT_DIR"

echo "Generating TypeScript SBOM (vscode-mdl)..."
cd "${ROOT_DIR}/vscode-mdl"
bun x @cyclonedx/cdxgen -t js -o "$TMP_TS" --no-recurse . 2>/dev/null

echo "Merging SBOMs..."
bun -e "
const go = require('${TMP_GO}');
const ts = require('${TMP_TS}');

// Use Go SBOM as base
const merged = { ...go };

// Merge TypeScript components (avoid duplicates by purl)
const existingPurls = new Set((go.components || []).map(c => c.purl).filter(Boolean));
const tsComponents = (ts.components || []).filter(c => !existingPurls.has(c.purl));
merged.components = [...(go.components || []), ...tsComponents];

// Update metadata
merged.metadata = merged.metadata || {};
merged.metadata.timestamp = new Date().toISOString();

// Update serial number
merged.serialNumber = 'urn:uuid:' + crypto.randomUUID();

// Add tools info
merged.metadata.tools = [
  { vendor: 'CycloneDX', name: 'cyclonedx-gomod' },
  { vendor: 'CycloneDX', name: 'cdxgen' }
];

const fs = require('fs');
fs.writeFileSync('${OUTPUT}', JSON.stringify(merged, null, 2) + '\n');

const goCount = (go.components || []).length;
const tsCount = tsComponents.length;
console.log('  Go components: ' + goCount);
console.log('  TypeScript components: ' + tsCount);
console.log('  Total: ' + (goCount + tsCount));
"

echo "Written to ${OUTPUT}"
