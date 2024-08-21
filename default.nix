{ lib, buildGoModule }:

buildGoModule {
  pname = "cliff";
  version = "0.1.0";

  src = ./.;

  vendorHash = "sha256-39B/CMwCGDJKsnm2GoCNpQRguST8kp43fv0cl3UFzi8=";
}
