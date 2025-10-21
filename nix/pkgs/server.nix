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
  pname = "msgscript";
  inherit version vendorHash;

  src = ../..;

  subPackages = [ "cmd/server" ];

  nativeBuildInputs = [ ] ++ (lib.optional withPodman [ pkg-config ]);

  buildInputs =
    [ ]
    ++ (lib.optional withWasm [ wasmtime.dev ])
    ++ (lib.optional withPodman [
      btrfs-progs
      gpgme
    ]);

  ldflags = [
    "-X"
    "main.version=${version}"
  ];

  tags = [ ] ++ (lib.optional withWasm [ "wasmtime" ]) ++ (lib.optional withPodman [ "podman" ]);

  doCheck = false; # Requires networking, will just timeout

  postInstall = ''
    mv $out/bin/server $out/bin/msgscript
  '';
}
