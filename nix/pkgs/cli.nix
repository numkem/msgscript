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

  nativeBuildInputs = [ ] ++ (lib.optional withPodman [ pkg-config ]);

  buildInputs =
    [ ]
    ++ (lib.optional withWasm [ wasmtime.dev ])
    ++ (lib.optional withPodman [
      btrfs-progs
      gpgme
    ]);

  postInstall = ''
    mv $out/bin/cli $out/bin/msgscriptcli
  '';
}
