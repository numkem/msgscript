{
  description = "Run Lua function from nats subjects";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      version = "0.1.6";
      vendorHash = "sha256-K5VF5qTrJ3Ia+f/X19xKLRgwcVjMmYqiIO3Ncgz+Vz4=";

      buildCli =
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        pkgs.buildGoModule {
          pname = "msgscript-cli";
          inherit version vendorHash;

          src = self;

          subPackages = [ "cmd/cli" ];

          postInstall = ''
            mv $out/bin/cli $out/bin/msgscriptcli
          '';
        };

      buildServer =
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        pkgs.buildGoModule {
          pname = "msgscript";
          inherit version vendorHash;

          src = self;

          subPackages = [ "cmd/server" ];

          doCheck = false; # Requires networking, will just timeout

          postInstall = ''
            mv $out/bin/server $out/bin/msgscript
          '';
        };

      mkOverlay = system: {
        default = (
          final: prev: {
            msgscript-cli = self.packages.${system}.cli;
            msgscript-server = self.packages.${system}.server;
          }
        );
      };
    in
    {
      packages.x86_64-linux = rec {
        cli = buildCli "x86_64-linux";
        server = buildServer "x86_64-linux";
        default = server;
      };
      packages.aarch64-linux = rec {
        cli = buildCli "x86_64-linux";
        server = buildServer "x86_64-linux";
        default = server;
      };

      devShells.x86_64-linux.default =
        let
          pkgs = import nixpkgs { system = "x86_64-linux"; };
        in
        pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            just
            etcd
            natscli
            nats-top

            # Server compose
            arion

            # LSPs
            gopls
            lua-language-server
          ];
        };

      overlays = {
        aarch64-linux = mkOverlay "aarch64-linux";
        x86_64-linux = mkOverlay "x86_64-linux";
      };

      nixosModules.default = import ./nix/modules/default.nix;
    };
}
