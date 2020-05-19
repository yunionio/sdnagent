#!/usr/bin/env bash

prog="$0"
state="$1"


__errmsg() {
	logger -t "$prog" "$*"
}

__errmsg "got state $state"

set -x
case "$state" in
	MASTER)
		/opt/yunion/bin/sdnagent --config /etc/yunion/sdnagent.conf 2>&1 | logger -t sdnagent-eipgw &
		__errmsg "sdnagent eipgw process group id $$"
		;;
	*)
		pkill --pidfile '/var/run/yunion-sdnagent-eipgw.pid'
		ovs-vsctl --if-exists del-br breip
		ovs-vsctl list-ports brvpc \
			| grep '^ev-' \
			| while read port; do \
				ovs-vsctl --if-exists del-port brvpc "$port"; \
			done
		;;
esac
