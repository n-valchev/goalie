{ lib, buildGoModule, git, makeWrapper }:

buildGoModule {
  pname = "goalie";
  version = "0.1.0";

  src = ./.;

  vendorHash = null;

  # The tests take about 90 s, and a flake update in a project that uses goalie
  # would wait for them. Run them with `go test ./...` before a push instead.
  doCheck = false;

  nativeBuildInputs = [ makeWrapper ];

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
