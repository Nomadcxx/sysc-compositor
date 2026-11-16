# Fedora 44 (COPR) and openSUSE Tumbleweed (OBS). Offline: Go modules come
# from the release's vendor tarball.
Name:           sysc-compositor
Version:        0.1.0
Release:        1%{?dist}
Summary:        A scrollable-tiling Wayland compositor built in Go
License:        GPL-3.0-or-later
URL:            https://github.com/Nomadcxx/sysc-compositor
Source0:        %{url}/archive/refs/tags/v%{version}/%{name}-%{version}.tar.gz
Source1:        %{url}/releases/download/v%{version}/%{name}-%{version}-vendor.tar.xz

%if 0%{?suse_version}
BuildRequires:  go >= 1.26.4
%else
BuildRequires:  golang >= 1.26.4
%endif
BuildRequires:  gcc
BuildRequires:  make
BuildRequires:  pkgconfig(glesv2)
BuildRequires:  pkgconfig(libdrm)
BuildRequires:  pkgconfig(libinput)
BuildRequires:  pkgconfig(libpipewire-0.3)
BuildRequires:  pkgconfig(pixman-1)
BuildRequires:  pkgconfig(scenefx-0.5)
BuildRequires:  pkgconfig(wayland-protocols)
BuildRequires:  pkgconfig(wayland-scanner)
BuildRequires:  pkgconfig(wayland-server)
BuildRequires:  pkgconfig(wlroots-0.20)
BuildRequires:  pkgconfig(xkbcommon)
BuildRequires:  systemd-rpm-macros
%if 0%{?suse_version}
Requires:       dbus-service
%else
Requires:       dbus
%endif
Requires:       xkeyboard-config
Recommends:     xdg-desktop-portal-gnome
Recommends:     xdg-desktop-portal-gtk
Suggests:       xwayland-satellite

%description
A scrollable-tiling Wayland compositor built in Go, on wlroots 0.20 and
SceneFX, for the sysc desktop.

%prep
%autosetup -p1
tar -xJf %{SOURCE1}

%build
export GOTOOLCHAIN=local
export GOFLAGS="-mod=vendor -buildmode=pie -trimpath"
export CGO_CFLAGS="%{?build_cflags}"
export CGO_LDFLAGS="%{?build_ldflags}"
%make_build build VERSION=%{version}

%install
make install PREFIX=%{_prefix} DESTDIR=%{buildroot}
# %%license and %%doc install these themselves.
rm -r %{buildroot}%{_datadir}/licenses %{buildroot}%{_datadir}/doc

%files
%license LICENSE NOTICE
%doc README.md
%{_bindir}/sysc-compositor
%{_bindir}/sysc-compositor-session
%{_datadir}/wayland-sessions/sysc-compositor.desktop
%{_userunitdir}/sysc-compositor.service
%{_userunitdir}/sysc-compositor-shutdown.target
%dir %{_datadir}/xdg-desktop-portal
%{_datadir}/xdg-desktop-portal/sysc-portals.conf
%{_datadir}/sysc-compositor/

%changelog
* Mon Nov 16 2026 Nomadcxx <noovie@gmail.com> - 0.1.0-1
- First release.
