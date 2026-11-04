{ lib, buildGoModule, git, makeWrapper }:

buildGoModule {
  pname = "godry";
  version = "0.1.0";

  src = ./.;

  vendorHash = null;

  # Do not set subPackages: buildGoModule would then run only the tests of the
  # listed packages, and the tests live under internal/.
  nativeBuildInputs = [ makeWrapper ];
  nativeCheckInputs = [ git ];

  # --suffix, not --prefix: a git the user already has must win.
  postInstall = ''
    wrapProgram $out/bin/godry --suffix PATH : ${lib.makeBinPath [ git ]}
  '';

  meta = {
    description = "Finds Go functions with the same structure";
    homepage = "https://github.com/n-valchev/godry";
    license = lib.licenses.bsd0;
    mainProgram = "godry";
    platforms = lib.platforms.unix;
  };
}
