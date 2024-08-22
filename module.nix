{ self }: # needed to reference the package from the flake's output
{ config, lib, pkgs, ... }:

let
  cfg = config.services.cliff;
in
{
  options.services.cliff = {
    enable = lib.mkEnableOption "cliff, which lets you trigger notifications to your iPhone";

    package = lib.mkPackageOption self.packages.${pkgs.system} "cliff" {
      pkgsText = "inputs.cliff-server.packages.\${pkgs.system}";
    };

    hostname = lib.mkOption {
      type = lib.types.str;
      default = "cliff";
      description = "Hostname that cliff will use when connecting to your Tailnet";
    };

    apnsKeyPath = lib.mkOption {
      type = with lib.types; nullOr path;
      default = null;
      example = "/var/lib/cliff/AuthKey.p8";
      description = ''
        The path to the APNs token signing key.

        You must either provide a value for this option
        or set the `CLIFF_APNS_KEY_PATH` environment variable
        in the environment file.
      '';
    };

    development = lib.mkOption {
      type = lib.types.bool;
      default = false;
      example = true;
      description = ''
        Whether to hit the APNs development endpoint instead of the production endpoint.

        Builds installed directly via Xcode send APNs tokens that must hit the development
        endpoint, while builds installed via TestFlight or the App Store send APNs tokens
        that must hit the production endpoint.
      '';
    };

    environmentFile = lib.mkOption {
      type = lib.types.path;
      default = "/etc/cliff.env";
      example = "/var/lib/cliff.env";
      description = ''
        Additional environment file as defined in {manpage}`systemd.exec(5)`.

        You must set the following environment variables in this file:
        - {env}`CLIFF_APNS_KEY_ID`
        - {env}`CLIFF_APNS_TEAM_ID`
        - {env}`CLIFF_APP_BUNDLE_ID`

        Also, if you do not set the `apnsKeyPath` option, you must set the
        {env}`CLIFF_APNS_KEY_PATH` environment variable as well.
      '';
    };

    user = lib.mkOption {
      type = lib.types.str;
      default = "cliff";
      description = "User account under which cliff runs";
    };

    group = lib.mkOption {
      type = lib.types.str;
      default = "cliff";
      description = "Group account under which cliff runs";
    };

    verbose = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Whether to enable verbose logging";
    };
  };

  config = lib.mkIf cfg.enable {
    users = {
      users."${cfg.user}" = {
        group = cfg.group;
        shell = pkgs.bashInteractive;
        home = "/var/lib/cliff";
        description = "user for cliff service";
        isSystemUser = true;
      };
      groups."${cfg.group}" = {};
    };

    systemd.services.cliff = {
      description = "cliff system service";
      wantedBy = [ "multi-user.target" ];

      serviceConfig = {
        User = cfg.user;
        Group = cfg.group;
        Type = "exec";
        Restart = "always";
        WorkingDirectory = "/var/lib/cliff";
        StateDirectory = "cliff";
        ExecStart = "${lib.getExe cfg.package} --hostname ${cfg.hostname} ${lib.optionalString (cfg.apnsKeyPath != null) "--apns-key ${cfg.apnsKeyPath}"} ${lib.optionalString cfg.development "--development"}";
        EnvironmentFile = [ cfg.environmentFile ];
      };
    };
  };
}
