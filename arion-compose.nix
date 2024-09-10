{
  project.name = "msgscript";

  services.etcd-nats =
    { ... }:
    {
      nixos.useSystemd = true;
      nixos.configuration = {
        services.etcd.enable = true;
        services.nats.enable = true;
      };
      service.useHostStore = true;
      service.ports = [
        "2379:2379"
        "4442:4442"
      ];
    };
}
