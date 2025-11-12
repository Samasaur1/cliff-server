{ buildGoModule }:

buildGoModule {
  pname = "cliff";
  version = "0.6.4";

  src = ./.;

  vendorHash = "sha256-J6Y53D1RKZv2Zqu2KlMdhY+23a9uBDvrYA661vCeM0o=";

  meta.mainProgram = "cliff";
}
