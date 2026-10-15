{ pkgs, ... }:

rec {
  helloLua = pkgs.writeText "hello.lua" (builtins.readFile ./../../examples/hello.lua);
  pathLua = pkgs.writeText "path.lua" (builtins.readFile ./path.lua);

  wasmLua = pkgs.writeText "wasm.lua" ''
    --* subject: funcs.wasm
    --* name: wasm
    --* html: true
    --* executor: wasm
    /var/lib/msgscript/scripts/c.wasm
  '';
  wasm = pkgs.stdenv.mkDerivation {
    name = "msgscript-wasm-c-example";

    buildInputs = [ pkgs.emscripten ];

    src = ../../examples/wasm/c;

    installPhase = ''
      mkdir -p $out/bin
      cp c.wasm $out/bin/c.wasm
    '';
  };

  helloImageName = "localhost/hello";
  helloImage = pkgs.dockerTools.buildImage {
    name = helloImageName;
    tag = "latest";
    copyToRoot = pkgs.buildEnv {
      name = "root";
      paths = [ pkgs.hello ];
      pathsToLink = [ "/bin" ];
    };
    config.Cmd = [ "/bin/hello" ];
  };

  podmanLua = pkgs.writeText "podman.lua" ''
    --* subject: funcs.podmanhello
    --* name: podman
    --* executor: podman
    ${builtins.toJSON { image = helloImageName; }}
  '';
}
