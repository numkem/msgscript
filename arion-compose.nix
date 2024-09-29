{ pkgs, ... }:

{
  project.name = "msgscript";

  services = {
    etcd = {
      image.enableRecommendedContents = true;
      service.useHostStore = true;
      service.command = [
        "${pkgs.etcd_3_5}/bin/etcd"
        "-advertise-client-urls"
        "http://127.1.1.1:2379"
        "-listen-client-urls"
        "http://0.0.0.0:2379"
      ];
      service.ports = [
        "2379:2379"
      ];
    };

    nats = {
      image.enableRecommendedContents = true;
      service.useHostStore = true;
      service.command = [ "${pkgs.nats-server}/bin/nats-server" ];
      service.ports = [ "4222:4222" ];
    };
  };
}
