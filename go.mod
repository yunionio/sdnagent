module yunion.io/x/sdnagent

go 1.12

require (
	github.com/ClickHouse/clickhouse-go v1.4.7 // indirect
	github.com/cheggaaa/pb/v3 v3.0.8 // indirect
	github.com/coreos/go-iptables v0.4.5
	github.com/coreos/go-systemd v0.0.0-20190620071333-e64a0ec8b42a // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/digitalocean/go-openvswitch v0.0.0-20190515160856-1141932ed5cf
	github.com/fsnotify/fsnotify v1.4.9
	github.com/golang/protobuf v1.5.2
	github.com/kr/pty v1.1.5 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mattn/go-sqlite3 v1.14.9 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/moul/http2curl v1.0.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.3.2
	github.com/tencentcloud/tencentcloud-sdk-go v3.0.135+incompatible // indirect
	github.com/tmc/grpc-websocket-proxy v0.0.0-20190109142713-0ad062ec5ee5 // indirect
	github.com/vishvananda/netlink v1.0.0
	github.com/vishvananda/netns v0.0.0-20191106174202-0a2b9b5464df
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200819165624-17cef6e3e9d5 // indirect
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5 // indirect
	golang.org/x/net v0.0.0-20210525063256-abc453219eb5
	golang.org/x/term v0.0.0-20210615171337-6886f2dfbf5b // indirect
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
	gopkg.in/go-playground/assert.v1 v1.2.1 // indirect
	gopkg.in/go-playground/validator.v9 v9.29.1 // indirect
	k8s.io/apimachinery v0.19.3 // indirect
	yunion.io/x/jsonutils v1.0.0
	yunion.io/x/log v1.0.0
	yunion.io/x/onecloud v0.0.0-20221025180617-23c44b1579fb
	yunion.io/x/pkg v1.0.1-0.20220630095420-9925accd7c5e
	yunion.io/x/sqlchemy v1.0.2 // indirect
)

replace github.com/digitalocean/go-openvswitch => github.com/yousong/go-openvswitch v0.0.0-20200422025222-6b2d502be872

replace (
	google.golang.org/grpc => google.golang.org/grpc v1.29.0
	k8s.io/api v0.15.8 => k8s.io/api v0.19.3
	k8s.io/apimachinery v0.15.8 => k8s.io/apimachinery v0.19.3
	k8s.io/client-go v0.15.8 => k8s.io/client-go v0.19.3
	k8s.io/cluster-bootstrap v0.15.8 => k8s.io/cluster-bootstrap v0.19.3
)
