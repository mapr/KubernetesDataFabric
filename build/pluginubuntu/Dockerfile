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

# Ubuntu Image Build
FROM ubuntu:latest
WORKDIR /tmp
# dl prereqs
RUN apt-get update; \
    apt-get install -y apt-utils sudo lsb-release; \
    DEBIAN_FRONTEND=noninteractive apt-get install --allow-unauthenticated -y wget perl syslinux openjdk-8-jdk
# Install MapR packages
RUN wget http://archive.mapr.com/tools/KubernetesDataFabric/v1.0.0/ubuntu/mapr-client-6.0.0.20180206041848.GA-20180206041848.amd64.deb; \
    wget http://archive.mapr.com/tools/KubernetesDataFabric/v1.0.0/ubuntu/mapr-posix-client-basic_6.0.0.20180206041848.GA-20180206041848_amd64.deb; \
    wget http://archive.mapr.com/tools/KubernetesDataFabric/v1.0.0/ubuntu/mapr-posix-client-platinum_6.0.0.20180206041848.GA-20180206041848_amd64.deb; \
    dpkg -i --force-overwrite /tmp/mapr-client-6.0.0.20180206041848.GA-20180206041848.amd64.deb; \
    dpkg -i --force-overwrite /tmp/mapr-posix-client-basic_6.0.0.20180206041848.GA-20180206041848_amd64.deb; \
    dpkg -i --force-overwrite /tmp/mapr-posix-client-platinum_6.0.0.20180206041848.GA-20180206041848_amd64.deb; \
    mkdir -p /tmp/lib; \
    cp -r /opt/mapr/lib/libfuse.* /tmp/lib; \
    cp -r /opt/mapr/lib/libMapRClient_c.* /tmp/lib; \
    cp -r /opt/mapr/lib/libMapRClient.* /tmp/lib; \
    cp /usr/lib/jvm/java-8-openjdk-amd64/jre/lib/amd64/server/libjvm.so /tmp/lib/libjvm.so; \
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

# Use Small Image. Main Docker Build
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
