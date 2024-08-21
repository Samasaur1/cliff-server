{ lib, buildGoModule }:

buildGoModule {
  pname = "cliff";
  version = "0.2.0";

  src = ./.;

  vendorHash = "sha256-39B/CMwCGDJKsnm2GoCNpQRguST8kp43fv0cl3UFzi8=";
}
