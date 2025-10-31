#!/bin/bash

usage() {
    echo "Usage: $0 [-m] [-c] [-d diff_base_file1 diff_base_file2] [-no-run] -- [oc-mirror args]"
    echo "  -m       Enable memory profiling (--mem-prof)"
    echo "  -c       Enable CPU profiling (--cpu-prof)"
    echo "  -d diff_base_file1 diff_base_file2"
    echo "           Specify two files for comparison with -diff_base"
    echo "  -no-run  Skip running oc-mirror"
    echo "  --       Delimit script arguments from oc-mirror arguments"
    echo "  [oc-mirror args]       Arguments to be passed to oc-mirror"
    exit 1
}

cleanup() {
    if [[ ! -z $MEM_PROF_PID ]]; then
        kill $MEM_PROF_PID
        wait $MEM_PROF_PID 2>/dev/null
    fi
    if [[ ! -z $CPU_PROF_PID ]]; then
        kill $CPU_PROF_PID
        wait $CPU_PROF_PID 2>/dev/null
    fi
    if [[ ! -z $PROF_DIFF_PID ]]; then
        kill $PROF_DIFF_PID
        wait $PROF_DIFF_PID 2>/dev/null
    fi
}

trap cleanup EXIT

MEM_PROF=""
CPU_PROF=""
PROF_DIFF_FILES=()
NO_RUN=false

while [[ $# -gt 0 ]]; do
    key="$1"
    case $key in
        -m)
            MEM_PROF="--mem-prof"
            shift
            ;;
        -c)
            CPU_PROF="--cpu-prof"
            shift
            ;;
        -d)
            PROF_DIFF_FILES=("$2" "$3")
            if [ ${#PROF_DIFF_FILES[@]} -ne 2 ]; then
                usage
            fi
            PROF_DIFF="-diff_base=${PROF_DIFF_FILES[0]} ${PROF_DIFF_FILES[1]}"
            shift
            shift
            shift
            ;;
        -no-run)
            NO_RUN=true
            shift
            ;;
        --)
            shift
            break
            ;;
        *)
            usage
            ;;
    esac
done

if [[ $NO_RUN == false && $# -eq 0 ]]; then
    usage
fi

if [[ $NO_RUN == false ]]; then
    ./bin/oc-mirror $@ $MEM_PROF $CPU_PROF
else
    echo "Skipping oc-mirror execution as per no-run option."
fi

if [[ ! -z $MEM_PROF ]]; then
    go tool pprof -http=:6775 mem.prof &
    MEM_PROF_PID=$!
fi

if [[ ! -z $CPU_PROF ]]; then
    go tool pprof -http=:6776 cpu.prof &
    CPU_PROF_PID=$!
fi

if [[ ! -z $PROF_DIFF ]]; then
    go tool pprof -http=:6777 $PROF_DIFF &
    PROF_DIFF_PID=$!
fi

wait
