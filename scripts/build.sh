#!/bin/sh
# Builds everything into ./dist:
#   - static binaries (CGO disabled: no glibc version problems on old systems)
#   - filedrop-agent_<ver>_<arch>.deb, filedrop-agent-<ver>-1.<rpmarch>.rpm,
#     filedrop-<ver>-linux-<arch>.tar.gz (generic installer)
#   - filedrop-directory-<ver>-linux-<arch>.tar.gz
#   - SHA256SUMS
#
# Usage:  scripts/build.sh            (all architectures, all package types
#                                      whose tools are installed)
#   VERSION=2.1.0 ARCHES="amd64" scripts/build.sh
#
# Needs: go, tar, gzip; dpkg-deb (for .deb), rpmbuild (for .rpm) - missing
# tools just skip that package type.

set -eu
cd "$(dirname "$0")/.."
ROOT="$(pwd)"

VERSION="${VERSION:-$(cat VERSION)}"
VERSION="${VERSION#v}"
ARCHES="${ARCHES:-amd64 arm64}"
REPO_URL="${REPO_URL:-https://github.com/OWNER/filedrop}"
MAINTAINER="${MAINTAINER:-filedrop maintainers <noreply@example.invalid>}"
EPOCH="${SOURCE_DATE_EPOCH:-0}"
DIST="$ROOT/dist"

rm -rf "$DIST"
mkdir -p "$DIST"

tar_reproducible() { # tar_reproducible <archive> <dir-parent> <dir-name>
    tar --sort=name --mtime="@$EPOCH" --owner=0 --group=0 --numeric-owner \
        -C "$2" -cf - "$3" | gzip -9n > "$1"
}

for arch in $ARCHES; do
    case "$arch" in
        amd64) rpmarch=x86_64 ;;
        arm64) rpmarch=aarch64 ;;
        *) echo "unsupported architecture $arch" >&2; exit 1 ;;
    esac
    echo "=== linux/$arch ==="

    bin="$DIST/work/bin-$arch"
    mkdir -p "$bin"
    for prog in filedrop-agent filedrop-directory; do
        CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath \
            -ldflags "-s -w -X main.version=$VERSION" -o "$bin/$prog" "./cmd/$prog"
    done

    # ---- staged root file system: the single source for deb, rpm and tar ----
    stage="$DIST/work/stage-$arch"
    install -D -m 0755 "$bin/filedrop-agent"                         "$stage/usr/bin/filedrop-agent"
    install -D -m 0755 packaging/common/filedrop-keygen              "$stage/usr/bin/filedrop-keygen"
    install -D -m 0755 packaging/common/filedrop-setup               "$stage/usr/sbin/filedrop-setup"
    install -D -m 0755 packaging/common/filedrop-desktop-link.sh     "$stage/usr/lib/filedrop/filedrop-desktop-link.sh"
    install -D -m 0755 packaging/common/filedrop-cleanup.sh          "$stage/usr/lib/filedrop/filedrop-cleanup.sh"
    for u in filedrop-agent.service filedrop-desktop.service filedrop-desktop.timer \
             filedrop-cleanup.service filedrop-cleanup.timer; do
        install -D -m 0644 "packaging/common/$u" "$stage/usr/lib/systemd/system/$u"
    done
    install -D -m 0644 packaging/common/filedrop.sshd.conf  "$stage/etc/ssh/sshd_config.d/filedrop.conf"
    install -D -m 0640 packaging/common/agent.conf.default  "$stage/etc/filedrop/agent.conf"
    install -D -m 0640 packaging/common/cleanup.conf.default "$stage/etc/filedrop/cleanup.conf"
    for d in LICENSE README.md README.ru.md; do
        install -D -m 0644 "$d" "$stage/usr/share/doc/filedrop-agent/$d"
    done

    # ---- tar.gz with a generic installer ----
    tdir="filedrop-$VERSION-linux-$arch"
    mkdir -p "$DIST/work/tar-$arch"
    rm -rf "$DIST/work/tar-$arch/$tdir"
    mkdir -p "$DIST/work/tar-$arch/$tdir"
    cp -a "$stage" "$DIST/work/tar-$arch/$tdir/rootfs"
    install -m 0755 packaging/tar/install.sh   "$DIST/work/tar-$arch/$tdir/install.sh"
    install -m 0755 packaging/tar/uninstall.sh "$DIST/work/tar-$arch/$tdir/uninstall.sh"
    install -m 0644 README.md LICENSE          "$DIST/work/tar-$arch/$tdir/"
    tar_reproducible "$DIST/$tdir.tar.gz" "$DIST/work/tar-$arch" "$tdir"

    # ---- directory service tarball ----
    ddir="filedrop-directory-$VERSION-linux-$arch"
    mkdir -p "$DIST/work/dir-$arch/$ddir"
    install -m 0755 "$bin/filedrop-directory"                   "$DIST/work/dir-$arch/$ddir/filedrop-directory"
    install -m 0644 packaging/directory/filedrop-directory.service "$DIST/work/dir-$arch/$ddir/"
    install -m 0755 packaging/directory/install.sh              "$DIST/work/dir-$arch/$ddir/install.sh"
    install -m 0644 LICENSE                                     "$DIST/work/dir-$arch/$ddir/"
    tar_reproducible "$DIST/$ddir.tar.gz" "$DIST/work/dir-$arch" "$ddir"

    # ---- deb ----
    if command -v dpkg-deb >/dev/null 2>&1; then
        deb="$DIST/work/deb-$arch"
        rm -rf "$deb"
        cp -a "$stage" "$deb"
        # Debian-family systems that are not usr-merged read units from /lib
        mkdir -p "$deb/lib/systemd/system"
        mv "$deb"/usr/lib/systemd/system/* "$deb/lib/systemd/system/"
        rmdir "$deb/usr/lib/systemd/system" "$deb/usr/lib/systemd"
        mkdir -p "$deb/DEBIAN"
        sed -e "s|@VERSION@|$VERSION|" -e "s|@ARCH@|$arch|" \
            -e "s|@MAINTAINER@|$MAINTAINER|" -e "s|@URL@|$REPO_URL|" \
            packaging/deb/control.in > "$deb/DEBIAN/control"
        cp packaging/deb/conffiles packaging/deb/postinst packaging/deb/prerm packaging/deb/postrm "$deb/DEBIAN/"
        chmod 0755 "$deb/DEBIAN/postinst" "$deb/DEBIAN/prerm" "$deb/DEBIAN/postrm"
        chmod 0644 "$deb/DEBIAN/conffiles" "$deb/DEBIAN/control"
        dpkg-deb --root-owner-group -Zgzip --build "$deb" "$DIST/filedrop-agent_${VERSION}_$arch.deb" >/dev/null
    else
        echo "dpkg-deb not found - skipping .deb"
    fi

    # ---- rpm ----
    if command -v rpmbuild >/dev/null 2>&1; then
        top="$DIST/work/rpm-$arch"
        mkdir -p "$top"
        rpmbuild -bb --quiet --target "$rpmarch" \
            --define "_topdir $top" \
            --define "pkgversion $VERSION" \
            --define "stagedir $stage" \
            --define "repourl $REPO_URL" \
            packaging/rpm/filedrop-agent.spec
        cp "$top"/RPMS/*/*.rpm "$DIST/"
    else
        echo "rpmbuild not found - skipping .rpm"
    fi

    cp "$bin/filedrop-agent" "$DIST/filedrop-agent-linux-$arch"
done

rm -rf "$DIST/work"
( cd "$DIST" && sha256sum -- * > SHA256SUMS )
echo
ls -la "$DIST"
