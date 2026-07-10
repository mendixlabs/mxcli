#!/usr/bin/env bash
# rebuild-mpr.sh — 重建 testdata/expr-checker/minimal.mpr
#
# 当需要升级 minimal.mpr 到新 Mendix 版本时使用。
# 创建一个全新的 blank 项目来替换 minimal.mpr + mprcontents/ + widgets/。
#
# 重要：新建的 blank 项目会丢失 MyFirstModule 中的自定义内容。
# 重建后需要用 mxcli 重新添加测试所需的实体/微流（如有）。
#
# Widget 精简策略（详见 rebuild-widgets.sh）：
#   editorPreview.js → 清空（mx check 不需要）
#   editorConfig.js  → 保留真实内容（mx check 用于 schema 验证）
#   *.js / *.mjs     → 清空（运行时代码）
#   *.xml            → 保留（widget 属性定义）
#   package.xml      → 保留（widget 注册）
#   HTML/License.txt → 删除（纯文档）
#
# mx check 通过条件：
#   - 0 CE0462（widget MPK 存在且版本匹配）
#   - 0 CE0463（widget schema 未变化）
#   - 可接受：CE6083（主题设计属性，Atlas Core 预存页面）
#
# 使用方式：
#   cd testdata/expr-checker
#   bash scripts/rebuild-mpr.sh [mendix-version] [mx-binary]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

MX_VERSION="${1:-11.6.6}"
MX_BIN="${2:-}"
if [ -z "$MX_BIN" ]; then
  MX_BIN=$(ls ~/.mxcli/mxbuild/"$MX_VERSION"/modeler/mx 2>/dev/null | head -1)
fi
if [ -z "$MX_BIN" ] || [ ! -x "$MX_BIN" ]; then
  echo "ERROR: mx binary for $MX_VERSION not found." >&2
  echo "Install with: mxcli setup mxbuild --version $MX_VERSION" >&2
  exit 1
fi

TMPDIR_BASE=$(mktemp -d)
trap "rm -rf $TMPDIR_BASE" EXIT

echo "Creating Mendix $MX_VERSION blank project..."
"$MX_BIN" create-project --app-name "minimal" --output-dir "$TMPDIR_BASE" 2>&1 | grep -E "Done|Error" || true

echo "Replacing minimal.mpr..."
cp "$TMPDIR_BASE/minimal.mpr" "$DEST_DIR/minimal.mpr"

echo "Replacing mprcontents/..."
rm -rf "$DEST_DIR/mprcontents"
cp -r "$TMPDIR_BASE/mprcontents" "$DEST_DIR/"

echo "Rebuilding widgets/..."
bash "$SCRIPT_DIR/rebuild-widgets.sh" "$MX_BIN" "$MX_VERSION"

echo ""
echo "minimal.mpr rebuilt for Mendix $MX_VERSION"
echo "Run 'mx check testdata/expr-checker/minimal.mpr' to verify"
echo "Then restore any custom MyFirstModule content with mxcli exec"
