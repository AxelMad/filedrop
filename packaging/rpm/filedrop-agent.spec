# Built by scripts/build.sh from a pre-staged root file system (no compiling
# happens here). Defines passed by the build script: pkgversion, stagedir, repourl.
%global debug_package %{nil}
%global _build_id_links none
%global __os_install_post %{nil}
# gzip payload: readable by old rpm versions as well
%define _binary_payload w9.gzdio

Name:           filedrop-agent
Version:        %{pkgversion}
Release:        1
Summary:        Automatic file hand-over from a computer to its room's interactive panel
License:        MIT
URL:            %{repourl}
AutoReqProv:    no

%description
filedrop moves files and folders that a user drops into the "shared folder"
of a laptop/PC to the interactive panel in the same room, with no further
interaction. One package for every machine: the role (sender or receiver) is
derived from the hostname via a configurable naming scheme. Transfer uses
scp/SFTP with a dedicated key into a chrooted, shell-less account.

Requires an OpenSSH client (and an OpenSSH server on the panels).

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}
cp -a %{stagedir}/. %{buildroot}/

%post
/usr/sbin/filedrop-setup --postinst || true
exit 0

%preun
if [ "$1" = "0" ]; then
    /usr/sbin/filedrop-setup --teardown >/dev/null 2>&1 || true
fi
exit 0

%postun
if [ "$1" = "0" ]; then
    systemctl daemon-reload >/dev/null 2>&1 || true
    systemctl reload sshd >/dev/null 2>&1 || systemctl reload ssh >/dev/null 2>&1 || true
    echo "filedrop-agent removed. /opt/filedrop, /etc/filedrop and the user 'filedrop' were left in place (they may hold undelivered files) - remove them by hand if you want."
fi
exit 0

%files
%attr(0755,root,root) /usr/bin/filedrop-agent
%attr(0755,root,root) /usr/bin/filedrop-keygen
%attr(0755,root,root) /usr/sbin/filedrop-setup
%attr(0755,root,root) /usr/lib/filedrop/filedrop-desktop-link.sh
%attr(0755,root,root) /usr/lib/filedrop/filedrop-cleanup.sh
%attr(0644,root,root) /usr/lib/systemd/system/filedrop-agent.service
%attr(0644,root,root) /usr/lib/systemd/system/filedrop-desktop.service
%attr(0644,root,root) /usr/lib/systemd/system/filedrop-desktop.timer
%attr(0644,root,root) /usr/lib/systemd/system/filedrop-cleanup.service
%attr(0644,root,root) /usr/lib/systemd/system/filedrop-cleanup.timer
%config(noreplace) %verify(not user group mode) /etc/ssh/sshd_config.d/filedrop.conf
%config(noreplace) %verify(not user group mode) /etc/filedrop/agent.conf
%config(noreplace) %verify(not user group mode) /etc/filedrop/cleanup.conf
%doc /usr/share/doc/filedrop-agent
