{
  pkgs,
  modules,
  overlays,
}:

let
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
in
pkgs.testers.runNixOSTest {
  name = "minimal-test";

  node.pkgsReadOnly = false;
  nodes.server =
    {
      config,
      pkgs,
      lib,
      ...
    }:
    {
      imports = [
        modules
      ];

      nixpkgs.overlays = overlays ++ [
        (prev: final: {
          msgscript-worker = final.msgscript-worker.override { withWasm = true; };
        })
      ];

      services.etcd = {
        enable = true;
        listenClientUrls = [ "http://127.0.0.1:2379" ];
        listenPeerUrls = [ "http://127.0.0.1:2380" ];
        initialCluster = [ "${config.services.etcd.name}=http://127.0.0.1:2380" ];
        initialClusterState = "new";
      };

      services.msgscript = {
        enable = true;
        backend = "etcd";
        etcdEndpoints = [ "http://127.0.0.1:2379" ];
        workerEnvironment = {
          PATH = lib.makeBinPath [
            pkgs.busybox # For sh
            pkgs.hello
          ];
        };
      };

      environment.systemPackages = with pkgs; [
        jq
        gnugrep
        coreutils
        msgscript-cli
      ];
    };

  testScript = ''
    start_all()

    # Setup script files
    server.copy_from_host("${wasmLua}", "/var/lib/msgscript/scripts/wasm.lua")
    server.copy_from_host("${wasm}/bin/c.wasm", "/var/lib/msgscript/scripts/c.wasm")

    server.wait_for_unit("etcd.service")

    server.succeed("msgscriptcli add /var/lib/msgscript/scripts/wasm.lua")

    server.wait_for_unit("msgscript.service")

    with subtest("wasm based script"):
        server.succeed("curl -s http://127.0.0.1:7643/funcs.wasm | jq -r '.[0].payload' | base64 -d | grep Hello")
  '';
}
