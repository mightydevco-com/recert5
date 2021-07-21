FROM golang:latest AS builder


ENV DEBIAN_FRONTEND=noninteractive 
RUN apt-get update
RUN apt-get install -y curl golang make git



RUN mkdir -p $GOPATH/src/github.com/operator-framework && \
    cd $GOPATH/src/github.com/operator-framework && \
    git clone https://github.com/operator-framework/operator-sdk && \ 
    cd operator-sdk && \
    git checkout master && \
    make install && \
    operator-sdk version

ADD ./go/src/operator $GOPATH/src/operator 

WORKDIR $GOPATH/src/operator

RUN ls -laF

RUN cd $GOPATH/src/operator && \
    make release

RUN ls -laF $GOPATH/bin


#######################################################3
## IMAGE
#######################################################3

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/operator \
    USER_UID=1001 \
    USER_NAME=operator

# install operator binary
COPY --from=builder /go/bin/operator ${OPERATOR}

COPY go/src/operator/build/bin /usr/local/bin

RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
