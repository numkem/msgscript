FROM nixos/nix:latest AS builder

# Copy our source and setup our working dir.
COPY . /tmp/build
WORKDIR /tmp/build

# Build our Nix environment
RUN nix \
    --extra-experimental-features "nix-command flakes" \
    --option filter-syscalls false \
    build .#server && \
    mv /tmp/build/result /tmp/build/server

RUN nix \
    --extra-experimental-features "nix-command flakes" \
    --option filter-syscalls false \
    build .#cli && \
    mv /tmp/build/result /tmp/build/cli

# Copy the Nix store closure into a directory. The Nix store closure is the
# entire set of Nix store values that we need for our build.
RUN mkdir /tmp/nix-store-closure
RUN cp -R $(nix-store -qR server/) /tmp/nix-store-closure
RUN cp -R $(nix-store -qR cli/) /tmp/nix-store-closure


FROM scratch

WORKDIR /app

COPY --from=builder /tmp/nix-store-closure /nix/store
COPY --from=builder /tmp/build/server /app
COPY --from=builder /tmp/build/cli /app

EXPOSE 7643

CMD [ "/app/bin/server" ]
