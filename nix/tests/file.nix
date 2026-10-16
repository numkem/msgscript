{
  pkgs,
  modules,
  overlays,
}:

let
  inherit (import ./fixtures.nix { inherit pkgs; })
    helloLua
    pathLua
    wasmLua
    wasm
    helloImage
    podmanLua
    ;
in
pkgs.testers.runNixOSTest {
  name = "msgscript-test-file";

  node.pkgsReadOnly = false;
  nodes.server = { pkgs, lib, ... }: {
    imports = [
      modules
    ];

    nixpkgs.overlays = overlays ++ [
      (prev: final: {
        msgscript-worker = final.msgscript-worker.override {
          withWasm = true;
          withPodman = true;
        };
      })
    ];

    services.msgscript = {
      enable = true;
      enablePodman = true;
      plugins = with pkgs.msgscript-plugins; [ gopher-lua-libs ];
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
    ];

    virtualisation.podman.enable = true;
  };

  testScript = ''
    start_all()

    # Setup script files
    server.copy_from_host("${helloLua}", "/var/lib/msgscript/scripts/hello.lua")
    server.copy_from_host("${pathLua}", "/var/lib/msgscript/scripts/path.lua")
    server.copy_from_host("${wasmLua}", "/var/lib/msgscript/scripts/wasm.lua")
    server.copy_from_host("${wasm}/bin/c.wasm", "/var/lib/msgscript/scripts/c.wasm")
    server.copy_from_host("${podmanLua}", "/var/lib/msgscript/scripts/podman.lua")

    server.wait_for_unit("msgscript.service")
    server.wait_for_open_port(7643)

    server.wait_for_unit("podman.socket")
    server.succeed("podman load < ${helloImage}")
    server.succeed("podman image exists localhost/hello:latest")

    with subtest("simple lua script"):
        server.succeed("curl -s http://127.0.0.1:7643/funcs.hello | jq -r '.[0].payload' | base64 -d | grep funcs.hello")

    with subtest("lua script with PATH change"):
        server.succeed("curl -s http://127.0.0.1:7643/funcs.path | jq -r '.[0].payload' | base64 -d | grep 'Hello'")

    with subtest("wasm based script"):
        server.succeed("curl -s http://127.0.0.1:7643/funcs.wasm | jq -r '.[0].payload' | base64 -d | grep Hello")

    with subtest("podman based hello script"):
        server.succeed("curl -s http://127.0.0.1:7643/funcs.podmanhello | jq -r '.[0].payload' | base64 -d | grep Hello")
  '';
}
