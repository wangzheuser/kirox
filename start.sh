#!/bin/bash
set -euo pipefail

# 切换到脚本所在目录，确保从任意路径执行都能使用项目根目录配置。
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$ROOT_DIR/frontend"
cd "$ROOT_DIR"

WAILS_VERSION="${WAILS_VERSION:-latest}"

# 使用进程级 Go 网络配置，避免修改用户全局 go env。
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.google.cn}"

# 输出统一加前缀，方便定位启动阶段。
log() {
  printf '[start] %s\n' "$*" >&2
}

# 明确失败原因，避免后续命令产生误导性报错。
fail() {
  printf '[start] 错误：%s\n' "$*" >&2
  exit 1
}

# 校验基础命令是否存在，系统级工具不在脚本内静默安装。
require_command() {
  local name="$1"
  local hint="$2"

  if ! command -v "$name" >/dev/null 2>&1; then
    fail "未找到 $name，$hint"
  fi
}

# 解析可用的 Wails 命令，兼容 PATH、GOPATH/bin 和手动指定路径。
resolve_wails_command() {
  local gopath

  if [ -n "${WAILS_BIN:-}" ]; then
    if [ -x "$WAILS_BIN" ] || command -v "$WAILS_BIN" >/dev/null 2>&1; then
      printf '%s\n' "$WAILS_BIN"
      return 0
    fi

    fail "WAILS_BIN 指向的 Wails 不可用：$WAILS_BIN"
  fi

  if command -v wails >/dev/null 2>&1; then
    command -v wails
    return 0
  fi

  gopath="$(go env GOPATH)"
  if [ -x "$gopath/bin/wails" ]; then
    printf '%s\n' "$gopath/bin/wails"
    return 0
  fi

  return 1
}

# 缺少 Wails 时自动通过 Go 安装当前项目需要的开发工具。
ensure_wails() {
  local wails_cmd

  if wails_cmd="$(resolve_wails_command)"; then
    printf '%s\n' "$wails_cmd"
    return 0
  fi

  log "未找到 Wails，开始安装 github.com/wailsapp/wails/v2/cmd/wails@$WAILS_VERSION"
  go install "github.com/wailsapp/wails/v2/cmd/wails@$WAILS_VERSION"

  if wails_cmd="$(resolve_wails_command)"; then
    printf '%s\n' "$wails_cmd"
    return 0
  fi

  fail "Wails 安装完成后仍不可用，请检查 GOPATH/bin 权限或 PATH 配置"
}

# 下载 Go 模块，确保 Wails 启动前后端依赖完整。
ensure_go_dependencies() {
  if [ ! -f "$ROOT_DIR/go.mod" ]; then
    return 0
  fi

  log "检查 Go 依赖，GOTOOLCHAIN=$GOTOOLCHAIN，GOPROXY=$GOPROXY"
  go mod download
}

# 判断前端是否声明了 npm 依赖，避免无依赖项目每次重复 npm install。
has_frontend_dependencies() {
  node -e '
const pkg = require(process.argv[1]);
const sections = ["dependencies", "devDependencies", "optionalDependencies", "peerDependencies"];
process.exit(sections.some((section) => pkg[section] && Object.keys(pkg[section]).length > 0) ? 0 : 1);
' "$FRONTEND_DIR/package.json"
}

# 安装缺失或过期的前端依赖。
ensure_frontend_dependencies() {
  if [ ! -f "$FRONTEND_DIR/package.json" ]; then
    return 0
  fi

  require_command "node" "请先安装 Node.js"
  require_command "npm" "请先安装 npm"

  if ! has_frontend_dependencies; then
    log "前端未声明 npm 依赖，跳过 npm install"
    return 0
  fi

  if [ ! -d "$FRONTEND_DIR/node_modules" ] \
    || [ "$FRONTEND_DIR/package.json" -nt "$FRONTEND_DIR/node_modules" ] \
    || { [ -f "$FRONTEND_DIR/package-lock.json" ] && [ "$FRONTEND_DIR/package-lock.json" -nt "$FRONTEND_DIR/node_modules" ]; }; then
    log "安装前端依赖"
    (cd "$FRONTEND_DIR" && npm install)
  else
    log "前端依赖已就绪"
  fi
}

require_command "go" "请先安装 Go"
ensure_go_dependencies
ensure_frontend_dependencies
WAILS_CMD="$(ensure_wails)"

# 使用 Wails 开发模式启动项目，并强制重新构建 Go 后端，避免复用旧 dev 产物。
log "使用 Wails 开发模式启动项目（强制重新构建后端）"
exec "$WAILS_CMD" dev -forcebuild "$@"
