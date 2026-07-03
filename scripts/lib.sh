#!/bin/bash

os_name() {
    uname -s
}

is_root() {
    [ "${EUID:-$(id -u)}" -eq 0 ]
}

require_root() {
    action="$1"
    shift || true
    if ! is_root; then
        echo "[ERROR] $action 需要 root 权限，请使用 sudo 调用:"
        echo "  sudo $0 $*"
        exit 1
    fi
}

file_size() {
    path="$1"
    stat -c%s "$path" 2>/dev/null || stat -f%z "$path" 2>/dev/null
}

loopback_if() {
    if tcpdump -D 2>/dev/null | grep -Eq '(^|[0-9]+\.)(lo0)([[:space:]]|$)'; then
        echo "lo0"
        return
    fi
    if tcpdump -D 2>/dev/null | grep -Eq '(^|[0-9]+\.)(lo)([[:space:]]|$)'; then
        echo "lo"
        return
    fi
    echo "any"
}

default_route_if() {
    target="${1:-8.8.8.8}"
    case "$(os_name)" in
        Darwin)
            route get "$target" 2>/dev/null | awk '/interface:/{print $2; exit}'
            ;;
        Linux)
            ip route get "$target" 2>/dev/null | awk '
                {
                    for (i = 1; i <= NF; i++) {
                        if ($i == "dev") {
                            print $(i + 1)
                            exit
                        }
                    }
                }'
            ;;
    esac
}

stop_port_listener() {
    port="$1"
    if [ "$(os_name)" = "Linux" ] && command -v fuser >/dev/null 2>&1; then
        fuser -k "$port/tcp" 2>/dev/null || true
        return
    fi
    if command -v lsof >/dev/null 2>&1; then
        pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
        [ -n "$pids" ] && kill $pids 2>/dev/null || true
    fi
}

run_with_timeout() {
    duration="$1"
    shift
    if command -v timeout >/dev/null 2>&1; then
        timeout "$duration" "$@"
        return
    fi

    "$@" &
    child_pid=$!
    (
        sleep "$duration"
        kill "$child_pid" 2>/dev/null || true
    ) &
    watchdog_pid=$!
    wait "$child_pid"
    status=$?
    kill "$watchdog_pid" 2>/dev/null || true
    wait "$watchdog_pid" 2>/dev/null || true
    return "$status"
}
