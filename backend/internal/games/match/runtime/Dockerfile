# Runtime image for untrusted bot code (internal/games/match.DockerLauncher). Python and a bare node
# binary copied in from the official node image, on top of python:3.12-slim so both starter-kit languages
# (python3 -u <entry>, node <entry>) work without pulling in a second full base image. python:3.12-slim is
# Debian-based and already ships libstdc++, which the copied node binary is dynamically linked against.
#
# /bot stays owned by root, mode 0755 (not chowned to the bot's uid 65534): DockerLauncher mounts an
# anonymous volume over it and runs the container with --read-only, so a fresh volume is initialized with
# this directory's ownership and mode. docker cp (run by the daemon, unconstrained by the container's own
# user) can still populate it before the bot starts, but the bot process itself - running as 65534, with no
# write bit for anyone but root - gets EACCES writing there at runtime, closing off /bot as a disk-fill
# vector right alongside the read-only rootfs. See the comment above DockerLauncher.Launch.
FROM python:3.12-slim
COPY --from=node:22-slim /usr/local/bin/node /usr/local/bin/node
RUN mkdir /bot && chmod 0755 /bot
WORKDIR /bot
USER 65534:65534
ENV PYTHONDONTWRITEBYTECODE=1 HOME=/tmp LANG=C.UTF-8
