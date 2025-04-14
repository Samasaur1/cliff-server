{ buildGoModule }:

buildGoModule {
  pname = "cliff";
  version = "0.6.1";

  src = ./.;

  vendorHash = "sha256-Fd5qslTNmFTpEaY37Yb0nRDSjGXJpr/D0Ux+6Kj4HT4=";

  meta.mainProgram = "cliff";
}
