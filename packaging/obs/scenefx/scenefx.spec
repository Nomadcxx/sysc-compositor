# SceneFX for openSUSE Tumbleweed, which does not ship it; sysc-compositor's
# OBS project builds it first. Split as Tumbleweed splits wlroots: the
# library's soname is unversioned (libscenefx-0.5.so), so the .so is runtime.
%define libname libscenefx-0_5
Name:           scenefx
Version:        0.5
Release:        0
Summary:        Drop-in replacement for the wlroots scene API with effects
License:        MIT
URL:            https://github.com/wlrfx/scenefx
Source0:        %{url}/archive/refs/tags/%{version}.tar.gz#/%{name}-%{version}.tar.gz
BuildRequires:  gcc
BuildRequires:  meson >= 1.3
BuildRequires:  pkgconfig(egl)
BuildRequires:  pkgconfig(gbm)
BuildRequires:  pkgconfig(glesv2)
BuildRequires:  pkgconfig(lcms2)
BuildRequires:  pkgconfig(libdrm) >= 2.4.129
BuildRequires:  pkgconfig(pixman-1) >= 0.43.0
BuildRequires:  pkgconfig(wayland-protocols)
BuildRequires:  pkgconfig(wayland-scanner)
BuildRequires:  pkgconfig(wayland-server)
BuildRequires:  pkgconfig(wlroots-0.20)
BuildRequires:  pkgconfig(xkbcommon) >= 1.8.0

%description
SceneFX replaces the wlroots scene graph API and adds blur, shadows and
rounded corners.

%package -n %{libname}
Summary:        Drop-in replacement for the wlroots scene API with effects

%description -n %{libname}
SceneFX replaces the wlroots scene graph API and adds blur, shadows and
rounded corners. This package holds the shared library.

%package devel
Summary:        Development files for SceneFX
Requires:       %{libname} = %{version}

%description devel
Headers and pkg-config file for building against SceneFX.

%prep
%autosetup -p1

%build
%meson -Dexamples=false -Dcolor-management=enabled
%meson_build

%install
%meson_install

%ldconfig_scriptlets -n %{libname}

%files -n %{libname}
%license LICENSE
%{_libdir}/libscenefx-0.5.so

%files devel
%doc README.md
%{_includedir}/scenefx-0.5/
%{_libdir}/pkgconfig/scenefx-0.5.pc

%changelog
