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
  name = "msgscript-test-etcd";

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
          msgscript-worker = final.msgscript-worker.override {
            withWasm = true;
            withPodman = true;
          };
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
        enablePodman = true;
        etcdEndpoints = [ "http://127.0.0.1:2379" ];
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
        msgscript-cli
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

    server.wait_for_unit("etcd.service")
    server.wait_for_open_port(2379)

    server.succeed("msgscriptcli add /var/lib/msgscript/scripts/hello.lua")
    server.succeed("msgscriptcli add /var/lib/msgscript/scripts/path.lua")
    server.succeed("msgscriptcli add /var/lib/msgscript/scripts/wasm.lua")
    server.succeed("msgscriptcli add /var/lib/msgscript/scripts/podman.lua")

    server.wait_for_unit("podman.socket")
    server.succeed("podman load < ${helloImage}")
    server.succeed("podman image exists localhost/hello:latest")

    server.wait_for_unit("msgscript.service")
    server.wait_for_open_port(7643)

    with subtest("lua hello script"):
        server.succeed("curl -s http://127.0.0.1:7643/funcs.hello | jq -r '.[0].payload' | base64 -d | grep funcs.hello")

    with subtest("lua script with PATH change"):
        server.succeed("curl -s http://127.0.0.1:7643/funcs.path | jq -r '.[0].payload' | base64 -d | grep 'Hello'")

    with subtest("wasm based hello script"):
        server.succeed("curl -s http://127.0.0.1:7643/funcs.wasm | jq -r '.[0].payload' | base64 -d | grep Hello")

    with subtest("podman based hello script"):
        server.succeed("curl -s http://127.0.0.1:7643/funcs.podmanhello | jq -r '.[0].payload' | base64 -d | grep Hello")
  '';
}
