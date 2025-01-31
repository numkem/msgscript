{
  config,
  lib,
  pkgs,
  ...
}:

with lib;
let
  cfg = config.services.msgscript;

  pluginDir = pkgs.symlinkJoin {
    name = "msgscript-server-plugins";
    paths = cfg.plugins;
  };
in
{
  options.services.msgscript = {
    enable = mkEnableOption "Enable the msgscript service";

    etcdEndpoints = mkOption {
      type = types.listOf types.str;
      default = [ "http://127.0.0.1:2379" ];
      description = mdDoc "Etcd endpoints to connect to";
    };

    backend = mkOption {
      type = types.enum [
        "etcd"
        "file"
      ];
      default = "file";
      description = "Backend to use to store/execute the functions from";
    };

    plugins = mkOption {
      type = types.listOf types.package;
      default = [ ];
      description = "Plugins to add to the server";
    };

    natsUrl = mkOption {
      type = types.str;
      default = "";
      description = mdDoc "Nats.io URL to connect to";
    };

    dataDir = mkOption {
      type = types.str;
      default = "/var/lib/msgscript";
      description = mdDoc "Directory available to msgscript-server for io operation. The server owns this directory";
    };

    scriptDir = mkOption {
      type = types.str;
      default = "${cfg.dataDir}/scripts";
    };

    libraryDir = mkOption {
      type = types.str;
      default = "${cfg.dataDir}/libs";
    };

    user = mkOption {
      type = types.str;
      default = "msgscript";
      description = "User account under which msgscript runs.";
    };

    group = mkOption {
      type = types.str;
      default = "msgscript";
      description = "Group under which msgscript runs.";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.msgscript = {
      description = "Run Lua function from nats subjects";
      restartIfChanged = true;

      serviceConfig = {
        ExecStart = "${pkgs.msgscript-server}/bin/msgscript -backend ${cfg.backend} -etcdurl ${lib.concatStringsSep "," cfg.etcdEndpoints} -natsurl ${cfg.natsUrl} -plugin ${pluginDir} -script ${cfg.scriptDir} -library ${cfg.libraryDir}";

        User = cfg.user;
        Group = cfg.group;
        WorkingDirectory = cfg.dataDir;
        RuntimeDirectory = cfg.dataDir;
        Restart = "on-failure";
        TimeoutSec = 15;

        # Security options:
        NoNewPrivileges = true;
        SystemCallArchitectures = "native";
        RestrictAddressFamilies = [
          "AF_INET"
          "AF_INET6"
        ];
        RestrictNamespaces = !config.boot.isContainer;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        ProtectControlGroups = !config.boot.isContainer;
        ProtectHostname = true;
        ProtectKernelLogs = !config.boot.isContainer;
        ProtectKernelModules = !config.boot.isContainer;
        ProtectKernelTunables = !config.boot.isContainer;
        LockPersonality = true;
        PrivateTmp = !config.boot.isContainer;
        PrivateDevices = true;
        PrivateUsers = true;
        RemoveIPC = true;

        SystemCallFilter = [
          "~@clock"
          "~@aio"
          "~@chown"
          "~@cpu-emulation"
          "~@debug"
          "~@keyring"
          "~@memlock"
          "~@module"
          "~@mount"
          "~@obsolete"
          "~@privileged"
          "~@raw-io"
          "~@reboot"
          "~@setuid"
          "~@swap"
        ];
        SystemCallErrorNumber = "EPERM";
      };

      wantedBy = [ "multi-user.target" ];
      after = [ "networking.target" ];
    };

    users.users = mkIf (cfg.user == "msgscript") {
      msgscript = {
        inherit (cfg) group;
        isSystemUser = true;
        home = "${cfg.user}";
      };
    };

    users.groups = mkIf (cfg.group == "msgscript") {
      msgscript = { };
    };
  };
}
