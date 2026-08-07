# Binary packaging spec: the release workflow compiles ssh-first inside a
# Fedora container and hands the result to rpmbuild as a tarball, so this spec
# only stages files. Building Go and npm inside %build would need network
# access, which rpmbuild does not grant.
#
#   rpmbuild -bb --define "_topdir $PWD/rpmbuild" --define "version 0.2.2" \
#            packaging/ssh-first.spec

Name:           ssh-first
Version:        %{version}
Release:        1%{?dist}
Summary:        SSH, SFTP and admin-panel workspace

License:        MIT
URL:            https://github.com/junet-1/sshfirst
Source0:        %{name}-%{version}-linux-x86_64.tar.gz

ExclusiveArch:  x86_64

# The GTK4 + WebKitGTK 6.0 pair the Wails v3 backend links against.
Requires:       gtk4
Requires:       webkitgtk6.0

Recommends:     rsync
Recommends:     gnome-keyring

# The binary is built with -trimpath -ldflags=-w -s and carries no build-id
# section rpm could extract debuginfo from.
%global debug_package %{nil}

%description
SSH First keeps every server you administrate in one native window:
interactive SSH terminals, an SFTP file browser, port forwarding, and the web
control panels of your machines as tabs next to each other. It imports hosts
from ~/.ssh/config, stores passwords in the desktop keyring rather than on
disk, and saves whole layouts as shareable JSON workspaces.

%prep
%setup -q -n %{name}-%{version}-linux-x86_64

%install
install -Dm755 bin/ssh-first %{buildroot}%{_bindir}/ssh-first

# The desktop file is named after the GTK application ID (org.wails.ssh_first)
# so the shell can match a running window to it.
install -Dm644 share/applications/org.wails.ssh_first.desktop \
  %{buildroot}%{_datadir}/applications/org.wails.ssh_first.desktop
install -Dm644 share/metainfo/org.wails.ssh_first.metainfo.xml \
  %{buildroot}%{_datadir}/metainfo/org.wails.ssh_first.metainfo.xml
install -Dm644 share/icons/hicolor/1024x1024/apps/ssh-first.png \
  %{buildroot}%{_datadir}/icons/hicolor/1024x1024/apps/ssh-first.png
install -Dm644 share/pixmaps/ssh-first.png \
  %{buildroot}%{_datadir}/pixmaps/ssh-first.png

# Start in the tray on login without opening the main window.
install -Dm644 share/autostart/ssh-first.desktop \
  %{buildroot}%{_sysconfdir}/xdg/autostart/ssh-first.desktop

%check
desktop-file-validate %{buildroot}%{_datadir}/applications/org.wails.ssh_first.desktop

%files
%license LICENSE
%doc README.md
%{_bindir}/ssh-first
%{_datadir}/applications/org.wails.ssh_first.desktop
%{_datadir}/metainfo/org.wails.ssh_first.metainfo.xml
%{_datadir}/icons/hicolor/1024x1024/apps/ssh-first.png
%{_datadir}/pixmaps/ssh-first.png
%config(noreplace) %{_sysconfdir}/xdg/autostart/ssh-first.desktop

%changelog
* Fri Aug 07 2026 Julian <juwolf11@googlemail.com> - 0.2.2-1
- Packaged from the GitHub release workflow.
