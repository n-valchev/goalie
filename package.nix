{ lib, buildGoModule, git, makeWrapper }:

buildGoModule {
  pname = "goalie";
  version = "0.1.0";

  src = ./.;

  vendorHash = null;

  # Do not set subPackages: buildGoModule would then run only the tests of the
  # listed packages, and the tests live under internal/.
  nativeBuildInputs = [ makeWrapper ];
  nativeCheckInputs = [ git ];

  # --suffix, not --prefix: a git the user already has must win.
  postInstall = ''
    for tool in $out/bin/*; do
      wrapProgram "$tool" --suffix PATH : ${lib.makeBinPath [ git ]}
    done
  '';

  meta = {
    description = "Checks that keep coding agents from adding poor code";
    homepage = "https://github.com/n-valchev/goalie";
    license = lib.licenses.bsd0;
    mainProgram = "goalie";
    platforms = lib.platforms.unix;
  };
}
