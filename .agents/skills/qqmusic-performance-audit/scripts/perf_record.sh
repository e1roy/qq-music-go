#!/usr/bin/env bash
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
PPROF_URL=${QQMUSIC_PPROF_URL:-http://127.0.0.1:6060}
RUNS_DIR=${QQMUSIC_PERF_RUNS:-"$ROOT/docs/performance/runs"}

die() {
  printf 'perf_record: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

need curl
need jq
need ps
need git

usage() {
  cat <<'EOF'
usage:
  perf_record.sh init <label>
  perf_record.sh snapshot <case-id> <phase> <pid> [note]
  perf_record.sh cpu <case-id> <seconds> <binary>
  perf_record.sh heap <case-id> <phase> <binary>
  perf_record.sh report [run-dir]

Set QQMUSIC_PERF_RUN to select a run directory after init.
EOF
}

run_dir() {
  if [[ -n ${QQMUSIC_PERF_RUN:-} ]]; then
    printf '%s\n' "$QQMUSIC_PERF_RUN"
  elif [[ -f docs/performance/runs/latest ]]; then
    cat docs/performance/runs/latest
  else
    die "QQMUSIC_PERF_RUN is unset"
  fi
}

safe_label() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9._-' '-'
}

safe_case() {
  printf '%s' "$1" | tr -cs 'A-Za-z0-9._-' '-'
}

unique_profile_stem() {
  local dir=$1 case_id=$2 phase=$3 stem suffix
  stem="$dir/profiles/$case_id-$phase"
  suffix=2
  while compgen -G "$stem*" >/dev/null; do
    stem="$dir/profiles/$case_id-$phase-$suffix"
    suffix=$((suffix + 1))
  done
  printf '%s\n' "$stem"
}

init_run() {
  [[ $# -eq 1 ]] || die "init needs a label"
  local label run_id dir latest
  label=$(safe_label "$1")
  run_id=$(date -u +%Y%m%dT%H%M%SZ)-${label}
  dir="$RUNS_DIR/$run_id"
  [[ ! -e $dir ]] || die "run already exists: $dir"
  mkdir -p "$dir/notes" "$dir/profiles" "$dir/screenshots"
  latest="$RUNS_DIR/latest"
  printf '%s\n' "$dir" >"$latest"
  jq -n \
    --arg run_id "$run_id" \
    --arg created_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg git_commit "$(git rev-parse HEAD)" \
    --arg git_status "$(git status --porcelain=v1)" \
    --arg host_os "$(uname -s)" \
    --arg host_arch "$(uname -m)" \
    --arg pprof_url "$PPROF_URL" \
    '{
      schema_version: 1,
      run_id: $run_id,
      created_at: $created_at,
      git_commit: $git_commit,
      git_dirty: ($git_status != ""),
      host_os: $host_os,
      host_arch: $host_arch,
      pprof_url: $pprof_url
    }' >"$dir/run.json"
  : >"$dir/metrics.jsonl"
  cat >"$dir/summary.md" <<EOF
# QQMusic 性能审计 ${run_id}

- 构建：
- 二进制：
- 窗口尺寸：
- 账号状态：
- 用户批准范围：

## 结论

待填写。

## 覆盖

由 \`perf_record.sh report\` 生成后补齐每个 case 的 passed/failed/skipped。

## 优化前后

待填写。

## 剩余风险

待填写。
EOF
  printf '%s\n' "$dir"
}

snapshot_run() {
  [[ $# -ge 3 && $# -le 4 ]] || die "snapshot needs case-id, phase, pid [note]"
  local dir case_id phase pid note now rss_kib cpu mem line
  dir=$(run_dir)
  case_id=$(safe_case "$1")
  phase=$(safe_case "$2")
  pid=$3
  note=${4:-}
  [[ -f $dir/metrics.jsonl ]] || die "not a run directory: $dir"
  kill -0 "$pid" 2>/dev/null || die "process is not running: $pid"
  now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  rss_kib=$(ps -o rss= -p "$pid" | awk 'NF {print $1; exit}')
  cpu=$(ps -o %cpu= -p "$pid" | awk 'NF {print $1; exit}')
  [[ -n $rss_kib ]] || die "cannot read RSS for pid $pid"
  mem=$(curl -fsS "$PPROF_URL/mem.json") || die "cannot read $PPROF_URL/mem.json"
  line=$(jq -cn \
    --argjson mem "$mem" \
    --arg run_id "$(jq -r .run_id "$dir/run.json")" \
    --arg case_id "$case_id" \
    --arg phase "$phase" \
    --arg captured_at "$now" \
    --argjson pid "$pid" \
    --argjson rss_kib "$rss_kib" \
    --argjson cpu "$cpu" \
    --arg note "$note" \
    '$mem.s[-1] as $s | {
      schema_version: 1,
      kind: "snapshot",
      run_id: $run_id,
      case_id: $case_id,
      phase: $phase,
      captured_at: $captured_at,
      pid: $pid,
      rss_bytes: ($rss_kib * 1024),
      heap_live_bytes: $s[3],
      heap_free_bytes: $s[4],
      goroutines: $s[5],
      process_cpu_percent: $cpu,
      note: $note
    }')
  printf '%s\n' "$line" >>"$dir/metrics.jsonl"
  printf '%s\n' "$line"
}

cpu_profile() {
  [[ $# -eq 3 ]] || die "cpu needs case-id, seconds, binary"
  local dir case_id seconds binary stem profile top duration samples line
  dir=$(run_dir)
  case_id=$(safe_case "$1")
  seconds=$2
  binary=$3
  [[ $seconds =~ ^[1-9][0-9]*$ ]] || die "seconds must be a positive integer"
  [[ -x $binary ]] || die "binary is not executable: $binary"
  stem=$(unique_profile_stem "$dir" "$case_id" settled)
  profile="$stem.cpu.pb.gz"
  top="$stem.cpu-top.txt"
  curl -fsS "$PPROF_URL/debug/pprof/profile?seconds=$seconds" -o "$profile"
  go tool pprof -top -cum -nodecount=40 "$binary" "$profile" >"$top"
  duration=$(grep -m1 '^Duration:' "$top" || true)
  samples=$(grep -m1 'Total samples' "$top" || true)
  line=$(jq -cn \
    --arg run_id "$(jq -r .run_id "$dir/run.json")" \
    --arg case_id "$case_id" \
    --argjson seconds "$seconds" \
    --arg profile_file "${profile#"$dir/"}" \
    --arg top_file "${top#"$dir/"}" \
    --arg duration_line "$duration" \
    --arg samples_line "$samples" \
    '{
      schema_version: 1,
      kind: "profile",
      profile_type: "cpu",
      run_id: $run_id,
      case_id: $case_id,
      phase: "settled",
      seconds: $seconds,
      profile_file: $profile_file,
      top_file: $top_file,
      duration_line: $duration_line,
      samples_line: $samples_line
    }')
  printf '%s\n' "$line" >>"$dir/metrics.jsonl"
  printf '%s\n' "$line"
}

heap_profile() {
  [[ $# -eq 3 ]] || die "heap needs case-id, phase, binary"
  local dir case_id phase binary stem profile inuse alloc line
  dir=$(run_dir)
  case_id=$(safe_case "$1")
  phase=$(safe_case "$2")
  binary=$3
  [[ -x $binary ]] || die "binary is not executable: $binary"
  stem=$(unique_profile_stem "$dir" "$case_id" "$phase")
  profile="$stem.heap.pb.gz"
  inuse="$stem.heap-inuse-top.txt"
  alloc="$stem.heap-alloc-top.txt"
  curl -fsS "$PPROF_URL/debug/pprof/heap?gc=1" -o "$profile"
  go tool pprof -top -nodecount=40 -inuse_space "$binary" "$profile" >"$inuse"
  go tool pprof -top -nodecount=40 -alloc_space "$binary" "$profile" >"$alloc"
  line=$(jq -cn \
    --arg run_id "$(jq -r .run_id "$dir/run.json")" \
    --arg case_id "$case_id" \
    --arg phase "$phase" \
    --arg profile_file "${profile#"$dir/"}" \
    --arg inuse_file "${inuse#"$dir/"}" \
    --arg alloc_file "${alloc#"$dir/"}" \
    '{
      schema_version: 1,
      kind: "profile",
      profile_type: "heap",
      run_id: $run_id,
      case_id: $case_id,
      phase: $phase,
      profile_file: $profile_file,
      inuse_top_file: $inuse_file,
      alloc_top_file: $alloc_file
    }')
  printf '%s\n' "$line" >>"$dir/metrics.jsonl"
  printf '%s\n' "$line"
}

report_run() {
  [[ $# -le 1 ]] || die "report takes at most one run directory"
  local dir out
  if [[ $# -eq 1 ]]; then
    dir=$1
  else
    dir=$(run_dir)
  fi
  [[ -f $dir/metrics.jsonl ]] || die "not a run directory: $dir"
  out="$dir/metrics.md"
  {
    printf '# Snapshot 记录\n\n'
    printf '| Case | Phase | RSS MiB | Go live MiB | Free MiB | Goroutines | CPU%% | Note |\n'
    printf '|---|---|---:|---:|---:|---:|---:|---|\n'
    jq -r 'select(.kind == "snapshot") |
      "| \(.case_id) | \(.phase) | \((.rss_bytes / 1048576 * 10 | round) / 10) | \((.heap_live_bytes / 1048576 * 10 | round) / 10) | \((.heap_free_bytes / 1048576 * 10 | round) / 10) | \(.goroutines) | \(.process_cpu_percent) | \(.note | gsub("\\|"; "/")) |"' \
      "$dir/metrics.jsonl"
    printf '\n# Profile 记录\n\n'
    printf '| Case | Type | Phase | Seconds | File | Summary |\n'
    printf '|---|---|---|---:|---|---|\n'
    jq -r 'select(.kind == "profile") |
      "| \(.case_id) | \(.profile_type) | \(.phase) | \(.seconds // "-") | \(.profile_file) | \((.samples_line // .inuse_top_file // "-") | gsub("\\|"; "/")) |"' \
      "$dir/metrics.jsonl"
  } >"$out"
  printf '%s\n' "$out"
}

cmd=${1:-}
case "$cmd" in
  init)
    shift
    init_run "$@"
    ;;
  snapshot)
    shift
    snapshot_run "$@"
    ;;
  cpu)
    shift
    cpu_profile "$@"
    ;;
  heap)
    shift
    heap_profile "$@"
    ;;
  report)
    shift
    report_run "$@"
    ;;
  -h | --help | help | "")
    usage
    ;;
  *)
    die "unknown command: $cmd"
    ;;
esac
