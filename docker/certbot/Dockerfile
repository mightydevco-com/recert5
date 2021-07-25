FROM certbot/certbot


RUN apk update && \
    apk upgrade && \
    apk add bash curl

RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/`curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt`/bin/linux/amd64/kubectl  && \
    mv kubectl /usr/bin && \
    chmod 755 /usr/bin/kubectl

RUN mkdir -p /opt/uberscott

WORKDIR /opt/uberscott

ENTRYPOINT /opt/uberscott/launcher.sh

COPY *.sh /opt/uberscott/
