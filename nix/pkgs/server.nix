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

  nativeBuildInputs = [ pkg-config ];

  buildInputs =
    [ ]
    ++ (lib.optional withWasm [ wasmtime.dev ])
    ++ (lib.optional withPodman [
      btrfs-progs
      gpgme
    ]);

  doCheck = false; # Requires networking, will just timeout

  postInstall = ''
    mv $out/bin/server $out/bin/msgscript
  '';
}
