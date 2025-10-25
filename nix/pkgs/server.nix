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

  postInstall = ''
    mv $out/bin/server $out/bin/msgscript
  '';
}
