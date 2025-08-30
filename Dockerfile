FROM nixos/nix:latest AS builder

# Copy our source and setup our working dir.
COPY . /tmp/build
WORKDIR /tmp/build

# Build our Nix environment
RUN nix \
    --extra-experimental-features "nix-command flakes" \
    --option filter-syscalls false \
    --option max-jobs auto \
    build .#server && \
    mv /tmp/build/result /tmp/build/server

RUN nix \
    --extra-experimental-features "nix-command flakes" \
    --option filter-syscalls false \
    --option max-jobs auto \
    build .#cli && \
    mv /tmp/build/result /tmp/build/cli

RUN nix \
    --extra-experimental-features "nix-command flakes" \
    --option filter-syscalls false \
    --option max-jobs auto \
    build .#allPlugins && \
    mv /tmp/build/result /tmp/build/allPlugins

# Copy the Nix store closure into a directory. The Nix store closure is the
# entire set of Nix store values that we need for our build.
RUN mkdir /tmp/nix-store-closure
RUN cp -R $(nix-store -qR server/) /tmp/nix-store-closure
RUN cp -R $(nix-store -qR cli/) /tmp/nix-store-closure
RUN cp -R $(nix-store -qR allPlugins/) /tmp/nix-store-closure


FROM alpine:latest

WORKDIR /app

COPY ./docker/entrypoint.sh /entrypoint.sh
RUN mkdir -p /app/share/plugins && chmod +x /entrypoint.sh

COPY --from=builder /tmp/nix-store-closure /nix/store
COPY --from=builder /tmp/build/server /app
COPY --from=builder /tmp/build/cli /app
COPY --from=builder /tmp/build/allPlugins/*.so /app/share/plugins/
RUN mv /app/bin/msgscript /app/bin/server && mv /app/bin/msgscriptcli /app/bin/cli

EXPOSE 7643

ENTRYPOINT ["/entrypoint.sh"]
