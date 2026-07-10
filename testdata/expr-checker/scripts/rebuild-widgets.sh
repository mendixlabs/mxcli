#!/usr/bin/env bash
# rebuild-widgets.sh — 重建 testdata/expr-checker/widgets/ 目录
#
# 用于：在 Mendix 版本升级或 minimal.mpr 重建后，用正确版本的 widget MPK 重填 widgets/。
# 原理：
#   1. 用 mx create-project 创建与 minimal.mpr 相同 Mendix 版本的 blank 项目
#   2. 从新项目的 widgets/ 目录中提取 MPK
#   3. 精简：清空 editorPreview.js、主 JS bundle、CSS、SVG/PNG 内容（减小体积）
#   4. 保留：package.xml、widget XML 定义、editorConfig.js（mx check 需要这些验证 schema）
#
# 精简规则（来自实测）：
#   KEEP 真实内容: *.xml, package.xml, *editorConfig.js, dependencies.json
#   CLEAR 内容为空:  *editorPreview.js, com/**/*.js/mjs, *.css, *.svg, *.png, dependencies.txt
#   DELETE 文件:   *.html (README/License), License.txt
#
# 使用方式：
#   cd testdata/expr-checker
#   bash scripts/rebuild-widgets.sh [mx-binary] [mendix-version]
#
# 参数：
#   mx-binary      可选，mx 工具路径，默认自动查找 ~/.mxcli/mxbuild/*/modeler/mx
#   mendix-version 可选，Mendix 版本号（如 11.6.6），默认从 minimal.mpr 读取
#
# 退出码：
#   0 = 成功，widgets/ 已更新
#   1 = 失败

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
WIDGETS_DIR="$DEST_DIR/widgets"

# --- 找 mx 工具 ---
MX_BIN="${1:-}"
if [ -z "$MX_BIN" ]; then
  MX_BIN=$(ls ~/.mxcli/mxbuild/*/modeler/mx 2>/dev/null | sort | tail -1)
fi
if [ -z "$MX_BIN" ] || [ ! -x "$MX_BIN" ]; then
  echo "ERROR: mx binary not found. Set MX_BINARY or pass path as first argument." >&2
  exit 1
fi
echo "Using mx: $MX_BIN"

# --- 获取 Mendix 版本 ---
MX_VERSION="${2:-}"
if [ -z "$MX_VERSION" ]; then
  # mx check exits non-zero when model has errors; capture output regardless
  MX_CHECK_OUT=$("$MX_BIN" check "$DEST_DIR/minimal.mpr" 2>&1) || true
  MX_VERSION=$(echo "$MX_CHECK_OUT" | grep -o "version is '.*'" | grep -o '[0-9][0-9.]*' | head -1)
fi
if [ -z "$MX_VERSION" ]; then
  echo "ERROR: Could not determine Mendix version from minimal.mpr" >&2
  exit 1
fi
echo "Mendix version: $MX_VERSION"

# --- 创建临时 blank 项目 ---
TMPDIR_BASE=$(mktemp -d)
trap "rm -rf $TMPDIR_BASE" EXIT

echo "Creating blank Mendix $MX_VERSION project..."
"$MX_BIN" create-project \
  --app-name "widget-ref" \
  --output-dir "$TMPDIR_BASE" \
  2>&1 | grep -E "Done|Error" || true

SRC_WIDGETS="$TMPDIR_BASE/widgets"
if [ ! -d "$SRC_WIDGETS" ]; then
  echo "ERROR: New project has no widgets/ directory" >&2
  exit 1
fi

COUNT=$(ls "$SRC_WIDGETS"/*.mpk 2>/dev/null | wc -l)
echo "Found $COUNT widget MPKs in new project"

# --- 精简并复制到目标 ---
echo "Slimming and copying to $WIDGETS_DIR..."
rm -f "$WIDGETS_DIR"/*.mpk

for SRC_MPK in "$SRC_WIDGETS"/*.mpk; do
  NAME=$(basename "$SRC_MPK")
  TMPD=$(mktemp -d)

  # 解压全部
  unzip -q "$SRC_MPK" -d "$TMPD" 2>/dev/null || true

  # 清空：editorPreview.js（大型预览渲染脚本，mx check 不需要）
  find "$TMPD" -name "*editorPreview.js" | while read f; do > "$f"; done

  # 清空：主 JS/MJS bundle（运行时代码，mx check 不需要验证 schema）
  find "$TMPD" -path "*/com/*/*.js" -o -path "*/com/*/*.mjs" | while read f; do > "$f"; done

  # 清空：CSS、SVG、PNG（仅用于渲染）
  find "$TMPD" -name "*.css" -o -name "*.svg" -o -name "*.png" | while read f; do > "$f"; done

  # 清空：依赖文本（不影响 schema）
  find "$TMPD" -name "dependencies.txt" | while read f; do > "$f"; done

  # 删除：HTML README/License（大文件，mx check 不需要）
  find "$TMPD" -name "*.html" -o -name "License.txt" | xargs rm -f 2>/dev/null || true

  # 重新打包
  (cd "$TMPD" && zip -q -r "$WIDGETS_DIR/$NAME" .)
  rm -rf "$TMPD"
done

FINAL_SIZE=$(du -sh "$WIDGETS_DIR" | cut -f1)
FINAL_COUNT=$(ls "$WIDGETS_DIR"/*.mpk 2>/dev/null | wc -l)
echo "Done: $FINAL_COUNT widgets, $FINAL_SIZE total"

# --- 验证 mx check ---
echo "Running mx check validation..."
RESULT=$("$MX_BIN" check "$DEST_DIR/minimal.mpr" 2>&1) || true
CE0462=$(echo "$RESULT" | grep -c "CE0462" || true)
CE0463=$(echo "$RESULT" | grep -c "CE0463" || true)

if [ "$CE0462" -gt 0 ] || [ "$CE0463" -gt 0 ]; then
  echo "VALIDATION FAILED:" >&2
  echo "$RESULT" | grep -E "CE0462|CE0463" | head -5 >&2
  exit 1
fi

echo "Validation passed: no CE0462/CE0463 errors"
