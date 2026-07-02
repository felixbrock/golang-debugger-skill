#!/bin/sh
# Run the full adoption grid (2 tasks x 2 conditions x 5 trials), appending one
# JSON line per run to results.jsonl. Control runs are parallelized; strong
# runs are sequential (each run's cleanup pkills gdbg daemons globally).
set -u
cd "$(dirname "$0")"
: > results.jsonl

run() { # cond idx task
    echo "[$(date +%H:%M:%S)] $3 $1 $2 ..." >&2
    python3 run_one.py "$1" "$2" "$3" >> results.jsonl
}

# control, 4 at a time
for task in accumulator rpncalc; do
    for i in 0 1 2 3 4; do
        [ "$task" = accumulator ] && [ "$i" = 0 ] && continue  # pilot done
        echo "$task $i"
    done
done | xargs -P 4 -n 2 sh -c 'cd "$(dirname "$0")"; python3 run_one.py control "$2" "$1" >> results.jsonl' "$0"

# strong, sequential
for task in accumulator rpncalc; do
    for i in 0 1 2 3 4; do
        [ "$task" = accumulator ] && [ "$i" = 0 ] && continue  # pilot done
        run strong "$i" "$task"
    done
done

echo "done" >&2
