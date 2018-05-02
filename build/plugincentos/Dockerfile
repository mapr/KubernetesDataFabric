# Copyright (c) 2009 & onwards. MapR Tech, Inc., All rights reserved

# Build Go Code
FROM golang:latest
RUN apt-get update; \
    apt-get install golang-glide
COPY maprfs/github.com /go/src/github.com
COPY maprfs/vendor /go/src/maprfs/vendor
COPY maprfs/glide.yaml /go/src/maprfs/glide.yaml
COPY maprfs/glide.lock /go/src/maprfs/glide.lock
COPY maprfs/main.go /go/src/maprfs/main.go
WORKDIR /go/src/maprfs
#RUN glide install -v
RUN go build

# CentOS Package Build
FROM centos:centos7
# Setup repos and dl prereqs
RUN rpm --import http://dl.fedoraproject.org/pub/epel/RPM-GPG-KEY-EPEL-7; \
    rpm --import https://www.centos.org/keys/RPM-GPG-KEY-CentOS-7; \
    yum -y update && yum -y clean all; \
    yum -y install epel-release; \
    sed -i 's/^mirror/#mirror/g' /etc/yum.repos.d/epel.repo; \
    sed -i 's/^#base/base/g' /etc/yum.repos.d/epel.repo; \
    yum install -y syslinux wget java-1.8.0-openjdk-devel
# Install MapR packages
WORKDIR /tmp

# Main Docker Build
RUN wget http://package.mapr.com/tools/KubernetesDataFabric/v1.0.1/redhat/mapr-client-6.0.1.20180404222005.GA-1.x86_64.rpm; \
    wget http://package.mapr.com/tools/KubernetesDataFabric/v1.0.1/redhat/mapr-librdkafka-0.11.3.201803231414-1.noarch.rpm; \
    wget http://package.mapr.com/tools/KubernetesDataFabric/v1.0.1/redhat/mapr-posix-client-basic-6.0.1.20180404222005.GA-1.x86_64.rpm; \
    wget http://package.mapr.com/tools/KubernetesDataFabric/v1.0.1/redhat/mapr-posix-client-platinum-6.0.1.20180404222005.GA-1.x86_64.rpm; \
    rpm -Uvh mapr-librdkafka-0.11.3.201803231414-1.noarch.rpm; \
    rpm -Uvh mapr-client-6.0.1.20180404222005.GA-1.x86_64.rpm; \
    rpm -Uvh mapr-posix-client-basic-6.0.1.20180404222005.GA-1.x86_64.rpm; \
    rpm -Uvh mapr-posix-client-platinum-6.0.1.20180404222005.GA-1.x86_64.rpm; \
    mkdir -p /tmp/lib; \
    cp -r /opt/mapr/lib/libfuse.* /tmp/lib; \
    cp -r /opt/mapr/lib/libMapRClient_c.* /tmp/lib; \
    cp -r /opt/mapr/lib/libMapRClient.* /tmp/lib; \
    cp /usr/lib/jvm/jre/lib/amd64/server/libjvm.so /tmp/lib/libjvm.so; \
    rm -rf /opt/mapr/contrib; \
    rm -rf /opt/mapr/conf; \
    rm -rf /opt/mapr/conf.new; \
    rm -rf /opt/mapr/examples; \
    rm -rf /opt/mapr/hadoop; \
    rm -rf /opt/mapr/initscripts; \
    rm -rf /opt/mapr/include; \
    rm -rf /opt/mapr/libexp; \
    rm -rf /opt/mapr/lib; \
    rm -rf /opt/mapr/logs; \
    rm -rf /opt/mapr/pid; \
    rm -rf /opt/mapr/server; \
    rm -rf /opt/mapr/bin/h*; \
    rm -rf /opt/mapr/bin/m*; \
    mkdir -p /opt/mapr/lib; \
    cp -r /tmp/lib/* /opt/mapr/lib; \
    rm -rf /tmp

# Use Small Image
FROM alpine:latest
# Add bash
RUN apk add --no-cache bash
# Copy MapR files
COPY --from=1 /opt/mapr /opt/mapr
# Copy script files
COPY copy2host /opt/mapr/plugin/copy2host
COPY copy2mapr /opt/mapr/plugin/copy2mapr
COPY start-fuse /opt/mapr/plugin/start-fuse
COPY pre-stop /opt/mapr/plugin/pre-stop
# Build GO code
COPY --from=0 /go/src/maprfs/maprfs /opt/mapr/plugin/maprfs
RUN chmod +x /opt/mapr/plugin/copy2host; \
    chmod +x /opt/mapr/plugin/copy2mapr; \
    chmod +x /opt/mapr/plugin/pre-stop; \
    chmod +x /opt/mapr/plugin/start-fuse; \
    chmod +x /opt/mapr/plugin/maprfs; \
    touch /opt/mapr/plugin/RELEASE; \
    ts=`stat -c %Y /opt/mapr/plugin/RELEASE`; \
    echo "$ts" > /opt/mapr/plugin/RELEASE
WORKDIR /opt/mapr/plugin/
CMD ["bash -c /opt/mapr/plugin/copy2host"]
