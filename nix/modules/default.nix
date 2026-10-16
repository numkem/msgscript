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
    name = "msgscript-worker-plugins";
    paths = cfg.plugins;
  };

  workerEnvironment = cfg.workerEnvironment // {
    XDG_RUNTIME_DIR = "/run/msgscript";
    HOME = dataDir;
  };

  workerEnvFile = pkgs.writeText "msgscript-worker.env" (
    lib.strings.join "\n" (lib.mapAttrsToList (k: v: "${k}=${toString v}") workerEnvironment)
  );

  dataDir = "/var/lib/msgscript";
in
{
  options.services.msgscript = {
    enable = mkEnableOption "Enable the msgscript service";

    etcdEndpoints = mkOption {
      type = types.listOf types.str;
      default = [ ];
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

    enablePodman = mkOption {
      type = types.bool;
      default = false;
      description = lib.mdDoc ''
        Enable podman for msgscript worker.

        This will only change the systemd service not the package build.

        The worker package needs to be overriden to include podman:
        ```
        (prev: final: {
          msgscript-worker = final.msgscript-worker.override { withPodman = yes; };
        })
        ```
      '';
    };

    enableTelemetry = mkOption {
      type = types.bool;
      default = false;
      description = "Whether to enable OpenTelemetry for msgscript.";
    };

    otelEndpoint = mkOption {
      type = types.str;
      default = "localhost:4317";
      description = "OpenTelemetry collector endpoint URL.";
    };

    workerEnvironment = mkOption {
      type = types.attrsOf types.str;
      default = {
        PATH = lib.makeBinPath [ pkgs.coreutils ];
      };
      description = "Environment variables passed to the worker";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.msgscript = {
      description = "Run Lua function from nats subjects";
      restartIfChanged = true;

      environment =
        { }
        // (optionalAttrs cfg.enableTelemetry {
          TELEMETRY_TRACES = "1";
          OTEL_ENDPOINT = cfg.otelEndpoint;
        });

      serviceConfig = {
        ExecStart =
          "${pkgs.msgscript-server}/bin/msgscript -backend ${cfg.backend} -plugin ${pluginDir} -script ${dataDir}/scripts -library ${dataDir}/libs -wexec ${pkgs.msgscript-worker}/bin/worker -wenv ${workerEnvFile}"
          + lib.optionalString (
            (lib.length cfg.etcdEndpoints) > 0
          ) " -etcdurl ${lib.concatStringsSep "," cfg.etcdEndpoints}"
          + lib.optionalString (cfg.natsUrl != "") " -natsurl ${cfg.natsUrl}";

        User = cfg.user;
        Group = cfg.group;
        SupplementaryGroups = if cfg.enablePodman then [ "podman" ] else [ ];
        Restart = "on-failure";
        RuntimeDirectory = "msgscript";
        StateDirectory = "msgscript";
        WorkingDirectory = dataDir;
        TimeoutSec = 15;

        # Security options:
        NoNewPrivileges = true;
        SystemCallArchitectures = "native";
        RestrictAddressFamilies = [
          "AF_INET"
          "AF_INET6"
        ]
        ++ lib.optionals cfg.enablePodman [ "AF_UNIX" ];
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
        home = dataDir;
      };
    };

    users.groups = mkIf (cfg.group == "msgscript") {
      msgscript = { };
    };

    systemd.tmpfiles.rules = [
      "d ${dataDir} 0775 ${cfg.user} ${cfg.group} -"
      "d ${dataDir}/libs 0775 ${cfg.user} ${cfg.group} -"
      "d ${dataDir}/scripts 0775 ${cfg.user} ${cfg.group} -"
    ];
  };
}
