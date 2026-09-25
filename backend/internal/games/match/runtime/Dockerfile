# Runtime image for untrusted bot code (internal/games/match.DockerLauncher). Python and a bare node
# binary copied in from the official node image, on top of python:3.12-slim so both starter-kit languages
# (python3 -u <entry>, node <entry>) work without pulling in a second full base image. python:3.12-slim is
# Debian-based and already ships libstdc++, which the copied node binary is dynamically linked against.
FROM python:3.12-slim
COPY --from=node:22-slim /usr/local/bin/node /usr/local/bin/node
RUN mkdir /bot && chown 65534:65534 /bot
WORKDIR /bot
USER 65534:65534
ENV PYTHONDONTWRITEBYTECODE=1 HOME=/tmp LANG=C.UTF-8
