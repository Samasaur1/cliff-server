{ buildGoModule }:

buildGoModule {
  pname = "cliff";
  version = "0.6.1";

  src = ./.;

  vendorHash = "sha256-GjSrYT+PMPUn23VaXtd+DPS+zAcIQDCvNcVpL8rcN6U=";

  meta.mainProgram = "cliff";
}
