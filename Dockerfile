# Runtime image only. Compile on the host first: sh deploy/build.sh
# Neither target pulls a new base image from Docker Hub.
# gateway is a static binary on scratch.
# console runs the prebuilt Next server with a Node binary downloaded on the host,
# on top of postgres:16, which is already present locally and provides glibc.

ARG CONSOLE_BASE=postgres:16

FROM scratch AS gateway
ARG GATEWAY_BINARY=bin/xhub-linux-amd64
COPY bin/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY ${GATEWAY_BINARY} /xhub
COPY configs/config.docker.yaml /etc/xhub/config.yaml
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
EXPOSE 4000
ENTRYPOINT ["/xhub", "-config", "/etc/xhub/config.yaml", "-addr", ":4000"]

FROM ${CONSOLE_BASE} AS console
USER root
WORKDIR /app
ARG NODE_BINARY=bin/node-linux-amd64
COPY ${NODE_BINARY} /usr/local/bin/node
COPY bin/console /app
ENV NODE_ENV=production
ENV XHUB_GATEWAY_ORIGIN=http://gateway:4000
ENV PORT=3000
ENV HOSTNAME=0.0.0.0
EXPOSE 3000
ENTRYPOINT ["node", "/app/server.js"]
