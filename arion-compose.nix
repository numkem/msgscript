{ pkgs, ... }:

{
  project.name = "msgscript";

  services = {
    etcd = {
      image.enableRecommendedContents = true;
      service.useHostStore = true;
      service.command = [ "${pkgs.etcd_3_5}/bin/etcd" ];
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
