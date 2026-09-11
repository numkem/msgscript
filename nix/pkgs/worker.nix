{
  lib,
  buildGoModule,
  btrfs-progs,
  gpgme,
  pkg-config,
  wasmtime,
  vendorHash,
  version,
  withPodman ? false,
  withWasm ? false,
}:

buildGoModule {
  pname = "msgscript-worker";
  inherit version vendorHash;

  src = ../..;

  subPackages = [ "cmd/worker" ];

  nativeBuildInputs = [ ] ++ (lib.optionals withPodman [ pkg-config ]);

  buildInputs =
    [ ]
    ++ (lib.optionals withWasm [ wasmtime.dev ])
    ++ (lib.optionals withPodman [
      btrfs-progs
      gpgme
    ]);

  ldflags = [
    "-X"
    "main.version=${version}"
  ];

  tags = [ ] ++ (lib.optionals withWasm [ "wasmtime" ]) ++ (lib.optionals withPodman [ "podman" ]);

  doCheck = false; # Requires networking, will just timeout
}
