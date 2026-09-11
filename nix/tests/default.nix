{
  pkgs,
  modules,
  overlays,
}:

{
  minimal-test = import ./minimal.nix { inherit pkgs modules overlays; };
}
