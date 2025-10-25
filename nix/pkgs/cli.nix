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
  pname = "msgscript-cli";
  inherit version vendorHash;

  src = ../..;

  subPackages = [ "cmd/cli" ];

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

  postInstall = ''
    mv $out/bin/cli $out/bin/msgscriptcli
  '';
}
