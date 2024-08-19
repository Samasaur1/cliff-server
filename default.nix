{ lib, buildGoModule }:

buildGoModule {
  pname = "cliff";
  version = "0.1.0";

  src = ./.;

  vendorHash = "sha256-msna/0BnNZSYXQWa/CXMC5oA1/vGLFJ9LDSx+YeD3GU=";
}
