{ lib, buildGoModule }:

buildGoModule {
  pname = "cliff";
  version = "0.1.0";

  src = ./.;

  vendorHash = "sha256-R1LD4Q8z5aaCiD1UWwJ5mWZw+OvKkIeLEQvORbYfefg=";
}
