{
  buildGoModule,
  vendorHash,
  version,
}:

buildGoModule {
  pname = "msgscript";
  inherit version vendorHash;

  src = ../..;

  subPackages = [ "cmd/server" ];

  ldflags = [
    "-X"
    "main.version=${version}"
  ];

  doCheck = false; # Requires networking, will just timeout

  postInstall = ''
    mv $out/bin/server $out/bin/msgscript
  '';
}
