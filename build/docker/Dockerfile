FROM registry.cn-beijing.aliyuncs.com/yunionio/openvswitch:2.12.4-4

ENV TZ=UTC
RUN mkdir -p /opt/yunion/bin
ADD ./_output/alpine-build/bin/sdnagent /opt/yunion/bin/sdnagent
ADD ./_output/alpine-build/bin/sdncli /opt/yunion/bin/sdncli
