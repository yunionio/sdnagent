#!/usr/bin/env bash

prog="$0"
statefile=/var/run/yunion-sdnagent-eipgw.state
pidfile=/var/run/yunion-sdnagent-eipgw.pid


__errmsg() {
	logger -t "$prog" "$*"
}

_start() {
	/opt/yunion/bin/sdnagent --config /etc/yunion/sdnagent.conf 2>&1 | logger -t sdnagent-eipgw &
	__errmsg "sdnagent eipgw process group id $$"
}

notify() {
	local state="$1"; shift

	__errmsg "got state $state"

	set -x
	echo "$state" >"$statefile"
	case "$state" in
		MASTER)
			_start
			;;
		*)
			pkill --pidfile "$pidfile"
			ovs-vsctl --if-exists del-br breip
			ovs-vsctl list-ports brvpc \
				| grep '^ev-' \
				| while read port; do \
					ovs-vsctl --if-exists del-port brvpc "$port"; \
				done
			;;
	esac
}

monitor() {
	local state
	if [ -s "$statefile" ]; then
		state="$(<"$statefile")"
		if [ "$state" = MASTER ]; then
			local comm
			comm="$(pgrep --list-name --pidfile "$pidfile" | cut -d' ' -f2)"
			if [ -z "$comm" -o "$comm" != sdnagent ]; then
				_start
			fi
		fi
	fi
	return 0
}

"$@"
