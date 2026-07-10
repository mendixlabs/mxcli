#!/usr/bin/env bash
# Generate a Markdown dependency report from sbom.cdx.json.
#
# Produces docs/DEPENDENCIES.md with a table of all dependencies and their licenses.
#
# Requires: bun, sbom.cdx.json (run `make sbom` first)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SBOM="${ROOT_DIR}/sbom.cdx.json"
OUTPUT="${ROOT_DIR}/docs/DEPENDENCIES.md"

if [ ! -f "$SBOM" ]; then
    echo "Error: $SBOM not found. Run 'make sbom' first."
    exit 1
fi

bun -e "
const fs = require('fs');
const d = require('${SBOM}');

const components = d.components || [];

function getLicense(c) {
    // Check direct licenses first
    if (c.licenses?.length > 0) {
        return c.licenses.map(l => l.license?.id || l.license?.name || l.expression || '').filter(Boolean).join(', ');
    }
    // Check evidence licenses (cyclonedx-gomod uses this)
    if (c.evidence?.licenses?.length > 0) {
        return c.evidence.licenses.map(l => l.license?.id || l.license?.name || '').filter(Boolean).join(', ');
    }
    return '';
}

function getType(c) {
    if (c.purl?.startsWith('pkg:golang')) return 'Go';
    if (c.purl?.startsWith('pkg:npm')) return 'npm';
    return c.type || '';
}

// Sort: Go first, then npm, alphabetically within each group
components.sort((a, b) => {
    const ta = getType(a), tb = getType(b);
    if (ta !== tb) return ta === 'Go' ? -1 : 1;
    const na = (a.group ? a.group + '/' : '') + a.name;
    const nb = (b.group ? b.group + '/' : '') + b.name;
    return na.localeCompare(nb);
});

const timestamp = d.metadata?.timestamp || new Date().toISOString();
const goCount = components.filter(c => getType(c) === 'Go').length;
const npmCount = components.filter(c => getType(c) === 'npm').length;

let md = '# Third-Party Dependencies\n\n';
md += 'Auto-generated from \`sbom.cdx.json\` — do not edit manually.  \n';
md += 'Regenerate with \`make sbom-report\`.\n\n';
md += '**Generated:** ' + timestamp.split('T')[0] + '  \n';
md += '**Total:** ' + components.length + ' dependencies (' + goCount + ' Go, ' + npmCount + ' npm)\n\n';

md += '| Ecosystem | Package | Version | License |\n';
md += '|-----------|---------|---------|----------|\n';

for (const c of components) {
    const ecosystem = getType(c);
    const name = (c.group ? c.group + '/' : '') + c.name;
    const version = c.version || '';
    const license = getLicense(c) || '—';
    md += '| ' + ecosystem + ' | ' + name + ' | ' + version + ' | ' + license + ' |\n';
}

md += '\n';

fs.writeFileSync('${OUTPUT}', md);
console.log('Written to ${OUTPUT}');
console.log('  ' + components.length + ' dependencies (' + goCount + ' Go, ' + npmCount + ' npm)');
const noLicense = components.filter(c => !getLicense(c));
if (noLicense.length > 0) {
    console.log('  ' + noLicense.length + ' without license info: ' + noLicense.map(c => c.name).join(', '));
}
"
