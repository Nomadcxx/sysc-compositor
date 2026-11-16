{
  description = "A scrollable-tiling Wayland compositor built in Go";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: {
        default = self.packages.${pkgs.stdenv.hostPlatform.system}.sysc-compositor;
        sysc-compositor = pkgs.buildGo126Module (finalAttrs: {
          pname = "sysc-compositor";
          version = "0.1.0";
          src = self;
          vendorHash = "sha256-Jkts2v/Ga8JBaklDuw5cYqkf/oXPEy2wSKNFmaX/mx8=";

          nativeBuildInputs = with pkgs; [
            pkg-config
            wayland-scanner
          ];
          buildInputs = with pkgs; [
            wlroots_0_20
            scenefx
            pipewire
            libxkbcommon
            pixman
            libdrm
            libinput
            wayland
            wayland-protocols
            libGL
          ];
          env.CGO_CFLAGS_ALLOW = "-fno-strict-overflow";

          # The Makefile is the one description of the build and the install.
          buildPhase = ''
            runHook preBuild
            make build VERSION=${finalAttrs.version}
            runHook postBuild
          '';
          installPhase = ''
            runHook preInstall
            make install PREFIX=$out
            runHook postInstall
          '';
          # The tests build compositors, which need a display.
          doCheck = false;

          passthru.providedSessions = [ "sysc-compositor" ];

          meta = {
            description = "A scrollable-tiling Wayland compositor built in Go";
            homepage = "https://github.com/Nomadcxx/sysc-compositor";
            license = pkgs.lib.licenses.gpl3Plus;
            mainProgram = "sysc-compositor";
            platforms = pkgs.lib.platforms.linux;
          };
        });
      });

      nixosModules.default =
        {
          config,
          lib,
          pkgs,
          ...
        }:
        let
          cfg = config.programs.sysc-compositor;
        in
        {
          options.programs.sysc-compositor = {
            enable = lib.mkEnableOption "the sysc-compositor Wayland session";
            package = lib.mkOption {
              type = lib.types.package;
              default = self.packages.${pkgs.stdenv.hostPlatform.system}.sysc-compositor;
              description = "The sysc-compositor package to use.";
            };
          };

          config = lib.mkIf cfg.enable {
            environment.systemPackages = [ cfg.package ];
            services.displayManager.sessionPackages = [ cfg.package ];
            # sysc-compositor-session starts the session through the user unit.
            systemd.packages = [ cfg.package ];
            hardware.graphics.enable = lib.mkDefault true;
            xdg.portal = {
              enable = lib.mkDefault true;
              configPackages = [ cfg.package ];
              # sysc-portals.conf names these backends.
              extraPortals = [
                pkgs.xdg-desktop-portal-gnome
                pkgs.xdg-desktop-portal-gtk
              ];
            };
          };
        };
    };
}
