{
  pkgs,
  modules,
  overlays,
}:

let
  helloLua = pkgs.writeText "hello.lua" (builtins.readFile ./../../examples/hello.lua);
  pathLua = pkgs.writeText "path.lua" (builtins.readFile ./path.lua);
in
pkgs.testers.runNixOSTest {
  name = "minimal-test";

  node.pkgsReadOnly = false;
  nodes.server = { pkgs, lib, ... }: {
    imports = [
      modules
    ];

    nixpkgs.overlays = overlays;

    services.msgscript = {
      enable = true;
      plugins = with pkgs.msgscript-plugins; [ gopher-lua-libs ];
      workerEnvironment = {
        PATH = lib.makeBinPath [
          pkgs.busybox # For sh
          pkgs.hello
        ];
      };
    };

    environment.systemPackages = with pkgs; [ jq gnugrep coreutils ];
  };

  skipLint = true;

  testScript = ''
    start_all()

    # Setup script files
    server.copy_from_host("${helloLua}", "/var/lib/msgscript/scripts/hello.lua")
    server.copy_from_host("${pathLua}", "/var/lib/msgscript/scripts/path.lua")

    server.wait_for_unit("msgscript.service")
    server.wait_for_open_port(7643)

    with subtest("simple lua script"):
        server.succeed("curl -s http://127.0.0.1:7643/funcs.hello | jq -r '.[0].payload' | base64 -d | grep funcs.hello")

    with subtest("lua script with PATH change"):
        server.succeed("curl -s http://127.0.0.1:7643/funcs.path | jq -r '.[0].payload' | base64 -d | grep 'Hello'")
  '';
}
