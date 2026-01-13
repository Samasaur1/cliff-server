{ buildGoModule }:

buildGoModule {
  pname = "cliff";
  version = "0.7.2";

  src = ./.;

  vendorHash = "sha256-MFrGev7KoZmEvOuLzsJ8ue906sclou5f3AvZbOVgWJg=";

  meta.mainProgram = "cliff";
}
