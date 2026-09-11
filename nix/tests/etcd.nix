{
  pkgs,
  modules,
  overlays,
}:

pkgs.testers.runNixOSTest {
  name = "minimal-test";

  node.pkgsReadOnly = false;
  nodes.server = { ... }: {
    imports = [
      modules
    ];

    nixpkgs.overlays = overlays;

    services = {
      etcd.enable = true;
      nats.enable = true;

      msgscript = {
        enable = true;
        etcdEndpoints = [ "http://127.0.0.1:2379" ];
        natsUrl = "localhost:4222";
      };
    };
  };

  skipLint = true;

  testScript = ''
    start_all()

    server.wait_for_unit("default.target")

    def print_command(cmd):
        output = server.succeed(cmd)
        print(output)

    print_command("journalctl -u msgscript")
    print_command("systemctl status msgscript")

    server.wait_for_open_port(2379)
    server.wait_for_open_port(4222)
    server.wait_for_open_port(7643)
  '';
}
