#!/bin/zsh
# Live scan monitor. Usage: ./tmp/watch.sh [interval_seconds]
DB=${DB:-tmp/byom.db}
LOG=${LOG:-tmp/run.log}
INT=${1:-5}
prev_clean=0; prev_t=0

while true; do
  line=$(sqlite3 -separator ' ' "$DB" "
    SELECT (SELECT COUNT(*) FROM dirs),
           (SELECT COUNT(*) FROM dirs WHERE dirty=0 AND locked_generation IS NULL),
           (SELECT COUNT(*) FROM dirs WHERE dirty=1),
           (SELECT COUNT(*) FROM dirs WHERE locked_generation IS NOT NULL),
           (SELECT COUNT(*) FROM albums),
           (SELECT COUNT(*) FROM tracks),
           (SELECT COUNT(*) FROM artists),
           (SELECT COUNT(*) FROM images),
           (SELECT COALESCE(state,'?') FROM scans ORDER BY start_time DESC LIMIT 1),
           (SELECT COALESCE(strftime('%s','now')-start_time,0) FROM scans ORDER BY start_time DESC LIMIT 1);" 2>/dev/null)
  [[ -z "$line" ]] && { sleep "$INT"; continue; }
  f=(${(s: :)line})
  dirs=$f[1] clean=$f[2] dirty=$f[3] locked=$f[4]
  albums=$f[5] tracks=$f[6] artists=$f[7] images=$f[8]
  state=$f[9] age=$f[10]

  now=$(date +%s)
  rate=0
  if (( prev_t > 0 && now > prev_t )); then
    rate=$(( (clean - prev_clean) * 1.0 / (now - prev_t) ))
  fi
  prev_clean=$clean; prev_t=$now

  remain=$(( dirty + locked ))
  eta="--"
  if (( rate > 0.01 )); then
    integer eta_i=$(( remain / rate ))
    eta=$(printf '%dm%02ds' $(( eta_i / 60 )) $(( eta_i % 60 )))
  fi

  warns=$(grep -c '"level":"WARN"' "$LOG" 2>/dev/null); warns=${warns:-0}
  errs=$(grep -c '"level":"ERROR"' "$LOG" 2>/dev/null); errs=${errs:-0}

  clear
  print -P "%B byom scan%b   walk=%F{cyan}${state}%f  elapsed=$(printf '%dm%02ds' $((age/60)) $((age%60)))"
  print    "─────────────────────────────────────────────"
  printf  " dirs walked   %6d\n" $dirs
  printf  " parsed        %6d   remaining %d (dirty %d, in-flight %d)\n" $clean $remain $dirty $locked
  printf  " parse rate    %6.2f dirs/s   eta %s\n" $rate $eta
  print    "─────────────────────────────────────────────"
  printf  " albums %-7d tracks %-8d artists %-6d images %d\n" $albums $tracks $artists $images
  printf  " warnings %-5d errors %d\n" $warns $errs
  [[ $warns -gt 0 ]] && { print "─── recent warnings ───"; grep '"level":"WARN"' "$LOG" | tail -3 | cut -c1-120 }
  sleep "$INT"
done
