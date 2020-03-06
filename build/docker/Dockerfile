FROM registry.cn-beijing.aliyuncs.com/yunionio/openvswitch:2.9.6-1

MAINTAINER "Yousong Zhou <zhouyousong@yunion.cn>"

RUN apk add iproute2 python2
RUN mkdir -p /opt/yunion/bin
ADD ./_output/alpine-build/bin/sdnagent /opt/yunion/bin/sdnagent
ADD ./_output/alpine-build/bin/sdncli /opt/yunion/bin/sdncli
