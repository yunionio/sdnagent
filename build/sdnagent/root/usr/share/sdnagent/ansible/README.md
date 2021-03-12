# 部署EIP网关

## 准备部署

首先，预留一个子网用来分配EIP.  该子网之后需要在OneCloud中注册为网络类型为eip的子网，EIP将从这个段中分配。

选定2台机器，用来部署EIP网关。如果只是测试用不需要主备高可用，只部署1台机器也可以。
EIP将会绑定到这2台机器上，因此要求外部网络设备将目的地址为EIP网段的流量路由到这两台机器。

EIP网关通过隧道与OneCloud宿主机通信，隧道协议为UDP，隧道外层IP为网关和宿主机的IP，通信路径中的防火墙需要放行此类流量。

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
