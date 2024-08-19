{
  description = "Listen for HTTP requests over Tailscale and send APNs pushes";

  inputs = {
  };

  outputs = { self, nixpkgs, ... }:
    let
      allSystems = nixpkgs.lib.systems.flakeExposed;
      forAllSystems = nixpkgs.lib.genAttrs allSystems;
      define = f: forAllSystems (system:
        let
          pkgs = import nixpkgs {
            inherit system;
            config = {
            };
          };
        in
          f pkgs
      );
    in {
      packages = define (pkgs: rec {
        cliff = pkgs.callPackage ./. {};
        default = cliff;
      });

      devShells = define (pkgs: {
        default = pkgs.mkShell {
          buildInputs = [ pkgs.go ];
        };
      });

      nixosModules.default = import ./module.nix;

      formatter = define (pkgs: pkgs.nixfmt-rfc-style);
    };
}
