# 部署EIP网关

## 准备部署

首先，预留一个子网用来分配EIP.  该子网之后需要在OneCloud中注册为网络类型为eip的子网，EIP将从这个段中分配。

选定2台机器，用来部署EIP网关。如果只是测试用不需要主备高可用，只部署1台机器也可以。
EIP将会绑定到这2台机器上，因此要求外部网络设备将目的地址为EIP网段的流量路由到这两台机器。

## 部署方式

EIP网关通过隧道与OneCloud宿主机通信，隧道协议为UDP，目的端口为6081，隧道报文外层IP为网关和宿主机的IP，通信路径中的防火墙需要放行此类流量。

在测试情况下，EIP网关可部署在平台的计算节点上，节省资源。另外，EIP网关也可部署到单独的宿主机、虚拟机中，只要满足上述的三层网络通信需求即可。

如果EIP网关部署在平台的计算节点上，计算节点运行了平台host pod，已经将openvswitch, ovn相关的组件都部署、配置好了，直接“实施部署”即可。

如果EIP网关部署在另外单独的节点上，则需要使用ISO中的.rpm安装包预先安装、配置好网关所需组件。以下对这部分进行描述

以3.6为例，所需安装的包的名字和所在位置如下

	# https://iso.yunion.cn/3.6/rpms/packages/kernel
	linux-firmware
	kernel

	# https://iso.yunion.cn/3.6/rpms/packages/host
	kmod-openvswitch
	unbound
	openvswitch
	openvswitch-ovn-common
	openvswitch-ovn-host

安装完内核之后，需要重启机器使之效

	# 启动openvswitch
	systemctl enable --now openvswitch

	# 配置ovn
	ovn_encap_ip=xx							# 隧道外层IP地址，EIP网关用它与其它计算节点通信
	ovn_north_addr=yy:32242					# ovn北向数据库的地址，yy一般选择某台宿主机ip地址；端口默认为32242，对应k8s default-ovn-north service中的端口号
	ovs-vsctl set Open_vSwitch . \
		external_ids:ovn-bridge=brvpc \
		external_ids:ovn-encap-type=geneve \
		external_ids:ovn-encap-ip=$ovn_encap_ip \
		external_ids:ovn-remote="tcp:$ovn_north_addr"
	# 启动ovn-controller
	systemctl enable --now ovn-controller

部署结束后，应当可以看到至少一个名为brvpc的openvswitch网桥，`ovs-vsctl show`命令的输出中可以看到名为ovn-xx，类型为geneve的隧道port，remote-ip指向计算节点的ovn-encap-ip

## 实施部署

部署采用ansible，过程概括为两步

 - 安装sdnagent, keepalived
 - 生成配置文件

将inventory文件复制一份，根据实际的环境，调整其中的变量值

	sdnagent_rpm					sdnagent.rpm在当前机器中的位置.  keepalived将从目标机器配置的yum仓库中直接部署

	oc_region						"oc_"前缀的变量用于向OneCloud keystone认证，访问OneCloud API服务。可以从default-climc pod
	oc_auth_url						通过"env | grep ^OS_"命令获得相应的值
	oc_admin_project
	oc_admin_user
	oc_admin_password

	vrrp_router_id					keepalived的virtual router id值。主备必须相同。若环境中有其他keepalived部署，必须不能冲突
	vrrp_priority					keepalived实例的priority，数值大的为MASTER，小的为BACKUP
	vrrp_interface					keepalived进行VRRP通信的网卡
	vrrp_vip						keepalived实例间相互通告的vip，可用作访问eip的下一跳地址

样例inventory的hosts描述了两台主机用作主备高可用，如果无需高可用，可将其中的一个主机描述删除，仅部署一台

inventory配置好以后，执行ansible playbook

	ansible-playbook -i a-inventory playbook.yaml
