FROM certbot/certbot


RUN apk update && \
    apk upgrade && \
    apk add bash curl

RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/`curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt`/bin/linux/amd64/kubectl  && \
    mv kubectl /usr/bin && \
    chmod 755 /usr/bin/kubectl

RUN mkdir -p /opt/mightydevco

WORKDIR /opt/mightydevco

ENTRYPOINT /opt/mightydevco/launcher.sh

COPY *.sh /opt/mightydevco/
