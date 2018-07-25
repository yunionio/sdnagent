# todo

6. failsafe trigger
7. more usable cmdline

	add-flow br1 cookie=0x99,priority=99,<mactch>,actions=

8. lock yunioncloud/pkg/log in Gopkg.toml
10. ping check on startup
14. unlink .sock on startup: systemd
15. After host_server
18. vlan_tci - dl_vlan
19. vlan = 1
21. encode who in cookie
22. intranet, external net
23. config file
24. vlan and ct zone allocation
26. match field, order by Name()
27. ovsdb port external_id
29. hostconfig with ct zone management, collision with ovn-controller?
30. set_queue for tc support
31. check availability of conntrack
25. cgo libopenvswitch
33. maybe, robustness, add logic to detect ct() , ct_state arguments order

34. TODO redirect broadcast ip traffic to sec_IN
35. TODO redirect LOCAL ip traffic to sec_IN
36. do we need to kill existing connection when new secrule applies
	- delete zone conntrack entries
37. conntrack entry timeout setting

# Test

Prepare dummy desc directory

- br0 in namespaces as physical hosts
- veth in namespace as virtual hosts

virtual hosts with single nic on the same host or different hosts

 - 2 on the same hosts
 - 2 on different hosts

virtual host with 2 nics enslaved to the same br0

 - 1 with 2 nics on different networks
 - 1 with the above as gateway in one of the network
 - 1 with the above as gateway in the other network

32. test ftp rel
20. regrestion test
38. nat for testing purposes
