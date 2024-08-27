{ buildGoModule }:

buildGoModule {
  pname = "cliff";
  version = "0.6.0";

  src = ./.;

  vendorHash = "sha256-IHJYFho0y7VJ0DJ6U/wlwy2q1/DbI/t5a6jkdvP0kxE=";

  meta.mainProgram = "cliff";
}
