# Copyright (c) 2009 & onwards. MapR Tech, Inc., All rights reserved
# Build Go Code
FROM golang:latest
RUN apt-get update; \
    apt-get install golang-glide
WORKDIR /go/src/maprfs-provisioner/
COPY maprfs-provisioner/github.com /go/src/github.com
COPY maprfs-provisioner/vendor /go/src/maprfs-provisioner/vendor
COPY maprfs-provisioner/glide.yaml /go/src/maprfs-provisioner/glide.yaml
COPY maprfs-provisioner/glide.lock /go/src/maprfs-provisioner/glide.lock
COPY maprfs-provisioner/main.go /go/src/maprfs-provisioner/main.go
#RUN glide install -v
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo

# Main Docker Build
FROM scratch
COPY --from=0 /go/src/maprfs-provisioner/maprfs-provisioner /maprfs-provisioner
WORKDIR /
CMD ["/maprfs-provisioner"]
