#!/usr/bin/env bash
# konflux-cross.sh - FIPS-capable cross builds for the Konflux CDN recipe (images/cli/Dockerfile.konflux).
#
# The CDN tarballs for all architectures are built on an x86_64 worker with `make cross-build`.
# This helper lets that unchanged make target run with CGO_ENABLED=1 and
# GOEXPERIMENT=strictfipsruntime, so v2 and the embedded v1 use the downstream Go OpenSSL backend.
#
#   prepare <el8|el9>  Verify and unpack the prefetched target RPMs into per-architecture sysroots,
#                      generate GOARCH-dispatching CC / PKG_CONFIG / cc wrappers, write
#                      ${OCM_CROSS_WORKDIR}/<el8|el9>/cross.env and print its path for the caller to source.
#   verify <el8|el9>   Run in the sourced environment after `make cross-build`. Fail unless every built
#                      binary (v2 and v1, all architectures) is a CGO, strictfipsruntime, dynamically
#                      linked build for the right architecture, and each v2 embeds the v1 built for the
#                      same architecture.
#
# Inputs (fail closed, there is no fallback to CGO_ENABLED=0):
#   - RPM files in ${OCM_CROSS_INPUT_DIR:-/cachi2/output/deps/generic} named
#       ocm-<el8|el9>-<aarch64|ppc64le|s390x>--<rpm>   target sysroot packages
#       ocm-<el8|el9>-x86_64-lld--<rpm>                 linker used for s390x only
#   - ${OCM_CROSS_LOCKFILE:-<repo>/artifacts.lock.yaml}, the generic artifact lockfile that ART generates
#     in the source root at rebase time from konflux.cachi2.artifact_lockfile in the image metadata.
#     Every ocm-<os>-* entry must be present with the recorded sha256, and no unlisted ocm-<os>-* file
#     may exist. The package list itself is maintained only in that image metadata.
#
# Toolchains: amd64 uses the builder gcc. arm64 and ppc64le use the builder clang and lld.
# s390x uses the builder clang and the prefetched lld (the builder lld 17 cannot link s390x;
# only ${OCM_CROSS_S390X_LLD_VERSION} has been tested).
set -euo pipefail
export GO_COMPLIANCE_INFO=0 # keep the ART Go wrapper quiet; this script only inspects and probes

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
INPUT_DIR=${OCM_CROSS_INPUT_DIR:-/cachi2/output/deps/generic}
LOCKFILE=${OCM_CROSS_LOCKFILE:-$ROOT/artifacts.lock.yaml}
WORKDIR=${OCM_CROSS_WORKDIR:-/tmp/oc-mirror-cross}
S390X_LLD_VERSION=${OCM_CROSS_S390X_LLD_VERSION:-21.1.8}
USAGE="usage: $0 prepare|verify <el8|el9>"
LLD_SET=x86_64-lld # input set name of the s390x linker RPMs

GOARCHES=(amd64 arm64 ppc64le s390x)
CROSS_GOARCHES=(arm64 ppc64le s390x)
declare -A RPMARCH=([arm64]=aarch64 [ppc64le]=ppc64le [s390x]=s390x)
declare -A TRIPLE=([arm64]=aarch64-redhat-linux-gnu [ppc64le]=powerpc64le-redhat-linux-gnu [s390x]=s390x-redhat-linux-gnu)
declare -A MACHINE=([amd64]="Advanced Micro Devices X86-64" [arm64]="AArch64" [ppc64le]="PowerPC64" [s390x]="IBM S/390")
declare -A INTERP=([amd64]=/lib64/ld-linux-x86-64.so.2 [arm64]=/lib/ld-linux-aarch64.so.1 [ppc64le]=/lib64/ld64.so.2 [s390x]=/lib/ld64.so.1)
declare -A GLIBC_MAX=([el8]=2.28 [el9]=2.34)
# Shared libraries the binaries may need; libc and libgpgme (cgo signature backend) are required.
# glibc 2.28 (RHEL 8) still ships libpthread and libdl separately; RHEL 9 merged them into libc.
declare -A ALLOWED_NEEDED=(
	[el9]=" libc.so.6 libgpgme.so.11 libassuan.so.0 libgpg-error.so.0 libresolv.so.2 "
	[el8]=" libc.so.6 libgpgme.so.11 libassuan.so.0 libgpg-error.so.0 libresolv.so.2 libpthread.so.0 libdl.so.2 "
)
REQUIRED_NEEDED=(libc.so.6 libgpgme.so.11)

log() { echo "konflux-cross: $*" >&2; }
die() { log "ERROR: $*"; exit 1; }
need() { local c; for c in "$@"; do command -v "$c" >/dev/null 2>&1 || die "required command not found: $c"; done; }
first_line() { printf '%s' "${1%%$'\n'*}"; }

check_os() {
	[[ ${1:-} == el8 || ${1:-} == el9 ]] || die "$USAGE"
}

# Print "<filename> <sha256>" for every ocm-<os>- entry of the ART generic artifact lockfile.
lockfile_entries() {
	local os=$1
	awk -v prefix="ocm-$os-" '
		function flush() { if (fn != "" && index(fn, prefix) == 1) print fn, sum; fn = ""; sum = "" }
		function val(s) { sub(/^[^:]*:[ \t]*/, "", s); gsub(/["\047 \t\r]/, "", s); return s }
		/^[ \t]*-[ \t]/ { flush(); sub(/^[ \t]*-[ \t]*/, "") }
		/^[ \t]*checksum:/ { sum = val($0); sub(/^sha256:/, "", sum) }
		/^[ \t]*filename:/ { fn = val($0) }
		END { flush() }
	' "$LOCKFILE"
}

# Rewrite absolute symlinks relative to the sysroot so they never resolve to build host files, and
# reject any symlink whose target leaves the sysroot. Runs after every RPM, so a later RPM can never
# be unpacked through a symlink left by an earlier one.
normalize_symlinks() {
	local root=$1 link target resolved
	while IFS= read -r -d '' link; do
		target=$(readlink "$link")
		ln -sfn "$(realpath -m -s --relative-to="$(dirname "$link")" "$root$target")" "$link"
	done < <(find "$root" -type l -lname '/*' -print0)
	while IFS= read -r -d '' link; do
		resolved=$(realpath -m -s "$(dirname "$link")/$(readlink "$link")")
		[[ $resolved == "$root" || $resolved == "$root"/* ]] || die "symlink ${link#"$root"} points outside the sysroot"
	done < <(find "$root" -type l -lname '*..*' -print0)
}

# Unpack one RPM payload without running scriptlets (the builders ship rpm2archive but not cpio).
unpack_rpm() {
	local rpmfile=$1 dest=$2
	rpm2archive <"$rpmfile" | tar --keep-directory-symlink -xzf - -C "$dest"
	chmod -R u+w "$dest"
	normalize_symlinks "$dest"
}

prepare() {
	local os=$1 w a e f sum set rpm_arch expected sr gccdir ldpath gcc clang lld pkgconfig key listing
	local gcc_v clang_v lld_v s390x_lld_v go_v
	local -a entries want_files
	local -A listed=()
	[[ $(uname -m) == x86_64 ]] || die "cross builds are designed for an x86_64 worker, found $(uname -m)"
	need go gcc clang ld.lld pkg-config rpm rpm2archive tar sha256sum realpath readelf awk
	[[ -f $LOCKFILE ]] || die "missing artifact lockfile $LOCKFILE (generated by ART from konflux.cachi2.artifact_lockfile)"
	[[ -d $INPUT_DIR ]] || die "missing prefetched input directory $INPUT_DIR"

	mapfile -t entries < <(lockfile_entries "$os")
	((${#entries[@]} > 0)) || die "no ocm-$os- entries in $LOCKFILE"
	listing=$(printf '%s\n' "${entries[@]}")
	for set in "${RPMARCH[@]}" "$LLD_SET"; do
		grep -q "^ocm-$os-$set--" <<<"$listing" || die "no ocm-$os-$set--* inputs listed in $LOCKFILE"
	done

	# Every listed input must be present with its recorded checksum; nothing unlisted may be used.
	for e in "${entries[@]}"; do
		f=${e%% *}; sum=${e##* }
		[[ $f == */* || $f == .* ]] && die "invalid input filename $f"
		[[ -f $INPUT_DIR/$f ]] || die "missing prefetched input $INPUT_DIR/$f"
		[[ $(sha256sum "$INPUT_DIR/$f" | cut -d' ' -f1) == "$sum" ]] || die "checksum mismatch for $f"
		listed[$f]=1
	done
	for f in "$INPUT_DIR"/ocm-"$os"-*; do
		[[ -e $f ]] || continue
		[[ -n ${listed[$(basename "$f")]:-} ]] || die "unlisted input $(basename "$f") is not in $LOCKFILE"
	done

	w=$WORKDIR/$os
	rm -rf "$w"
	mkdir -p "$w/sysroots" "$w/lld" "$w/bin"

	# Unpack the target sysroots and the s390x linker, refusing unknown sets and wrong-architecture RPMs.
	for e in "${entries[@]}"; do
		f=${e%% *}
		set=${f#"ocm-$os-"}; set=${set%%--*}
		case $set in
		"$LLD_SET") expected=x86_64; sr=$w/lld ;;
		aarch64 | ppc64le | s390x) expected=$set; sr=$w/sysroots/$set ;;
		*) die "unexpected input set '$set' in $f" ;;
		esac
		rpm_arch=$(rpm -qp --nosignature --nodigest --qf '%{ARCH}' "$INPUT_DIR/$f") || die "cannot read RPM header of $f"
		[[ $rpm_arch == "$expected" ]] || die "$f is a $rpm_arch package, expected $expected"
		if [[ ! -d $sr ]]; then
			mkdir -p "$sr/usr/lib" "$sr/usr/lib64" "$sr/usr/bin" "$sr/usr/sbin"
			ln -s usr/lib "$sr/lib"; ln -s usr/lib64 "$sr/lib64"; ln -s usr/bin "$sr/bin"; ln -s usr/sbin "$sr/sbin"
		fi
		unpack_rpm "$INPUT_DIR/$f" "$sr"
	done

	gcc=$(command -v gcc); clang=$(command -v clang); lld=$(command -v ld.lld); pkgconfig=$(command -v pkg-config)
	# shellcheck disable=SC2016 # the generated wrapper expands LD_LIBRARY_PATH and its arguments at run time
	printf '#!/bin/bash\nLD_LIBRARY_PATH=%q${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH} exec %q "$@"\n' \
		"$w/lld/usr/lib64" "$w/lld/usr/bin/ld.lld" >"$w/bin/s390x-ld.lld"
	chmod +x "$w/bin/s390x-ld.lld"
	s390x_lld_v=$("$w/bin/s390x-ld.lld" --version) || die "prefetched s390x linker does not run"
	s390x_lld_v=$(first_line "$s390x_lld_v")
	[[ $s390x_lld_v == "LLD $S390X_LLD_VERSION "* ]] || die "prefetched s390x linker is not LLD $S390X_LLD_VERSION: $s390x_lld_v"
	gcc_v=$("$gcc" --version) && gcc_v=$(first_line "$gcc_v")
	clang_v=$("$clang" --version) && clang_v=$(first_line "$clang_v")
	lld_v=$("$lld" --version) && lld_v=$(first_line "$lld_v")
	go_v=$(go env GOVERSION)

	# shellcheck disable=SC2016 # the generated wrappers expand $GOARCH and "$@" at run time
	{
		echo '#!/bin/bash'
		echo '# Generated by images/cli/konflux-cross.sh: C compiler for the GOARCH being built.'
		echo 'case "${GOARCH:-amd64}" in'
		printf 'amd64) exec %q "$@" ;;\n' "$gcc"
		for a in "${CROSS_GOARCHES[@]}"; do
			sr=$w/sysroots/${RPMARCH[$a]}
			gccdir=$(find "$sr/usr/lib/gcc/${RPMARCH[$a]}-redhat-linux" -mindepth 1 -maxdepth 1 -type d 2>/dev/null) || true
			[[ -n $gccdir && $gccdir != *$'\n'* ]] || die "expected one gcc install directory for $a in $sr/usr/lib/gcc"
			want_files=(usr/include/stdlib.h usr/include/gpgme.h usr/lib64/pkgconfig/gpgme.pc usr/lib64/libc.so usr/lib64/libgpgme.so "${INTERP[$a]#/}")
			for f in "${want_files[@]}"; do [[ -e $sr/$f ]] || die "$a sysroot lacks $f"; done
			for f in crtbegin.o libgcc.a libgcc_s.so; do [[ -e $gccdir/$f ]] || die "$a sysroot lacks ${gccdir#"$sr"}/$f"; done
			if [[ $a == s390x ]]; then ldpath=$w/bin/s390x-ld.lld; else ldpath=$lld; fi
			printf '%s) exec %q --target=%s --sysroot=%q --gcc-install-dir=%q --ld-path=%q -Qunused-arguments "$@" ;;\n' \
				"$a" "$clang" "${TRIPLE[$a]}" "$sr" "$gccdir" "$ldpath"
		done
		echo '*) echo "target-cc: unsupported GOARCH=$GOARCH" >&2; exit 1 ;;'
		echo 'esac'
	} >"$w/bin/target-cc"
	# shellcheck disable=SC2016 # the generated wrappers expand $GOARCH and "$@" at run time
	{
		echo '#!/bin/bash'
		echo '# Generated by images/cli/konflux-cross.sh: pkg-config for the GOARCH being built.'
		echo 'unset PKG_CONFIG_PATH PKG_CONFIG_SYSROOT_DIR PKG_CONFIG_LIBDIR'
		echo 'case "${GOARCH:-amd64}" in'
		printf 'amd64) exec %q "$@" ;;\n' "$pkgconfig"
		for a in "${CROSS_GOARCHES[@]}"; do
			sr=$w/sysroots/${RPMARCH[$a]}
			printf '%s) PKG_CONFIG_SYSROOT_DIR=%q PKG_CONFIG_LIBDIR=%q exec %q "$@" ;;\n' \
				"$a" "$sr" "$sr/usr/lib64/pkgconfig:$sr/usr/share/pkgconfig" "$pkgconfig"
		done
		echo '*) echo "target-pkg-config: unsupported GOARCH=$GOARCH" >&2; exit 1 ;;'
		echo 'esac'
	} >"$w/bin/target-pkg-config"
	chmod +x "$w/bin/target-cc" "$w/bin/target-pkg-config"
	# hack/*_tag.sh probe a bare `cc`; point it at the target compiler so build tags reflect the target.
	ln -s target-cc "$w/bin/cc"

	# Toolchain self-check: every target must compile and link a C program for the right machine.
	printf 'int main(void) { return 0; }\n' >"$w/selftest.c"
	for a in "${GOARCHES[@]}"; do
		GOARCH=$a "$w/bin/target-cc" "$w/selftest.c" -o "$w/selftest-$a" || die "C toolchain self-check failed for $a"
		[[ $(readelf -h "$w/selftest-$a") =~ Machine:\ +${MACHINE[$a]}$'\n' ]] || die "self-check binary for $a has the wrong machine"
		GOARCH=$a "$w/bin/target-pkg-config" --exists gpgme || die "gpgme pkg-config metadata not found for $a"
	done

	# Content-keyed build cache: always replaces any inherited GOCACHE, so cached cgo flags can never
	# come from a different sysroot, wrapper or toolchain.
	key=$(printf '%s\n' "$os" "$go_v" "$gcc_v" "$clang_v" "$lld_v" "$s390x_lld_v" |
		cat - "$w/bin/target-cc" "$w/bin/target-pkg-config" "$w/bin/s390x-ld.lld" <(printf '%s\n' "${entries[@]}" | sort) |
		sha256sum | cut -c1-16)

	{
		echo "# Generated by images/cli/konflux-cross.sh prepare $os"
		printf 'export OCM_CROSS_OS=%q\n' "$os"
		printf 'export CC=%q\n' "$w/bin/target-cc"
		printf 'export PKG_CONFIG=%q\n' "$w/bin/target-pkg-config"
		# shellcheck disable=SC2016 # PATH is expanded when the file is sourced
		printf 'export PATH=%q:"$PATH"\n' "$w/bin"
		echo 'export CGO_ENABLED=1'
		echo 'export GOEXPERIMENT=strictfipsruntime'
		echo 'export GOTOOLCHAIN=local'
		printf 'export GOCACHE=%q\n' "$WORKDIR/gocache/$os-$key"
		echo 'unset CGO_CFLAGS CGO_CPPFLAGS CGO_CXXFLAGS CGO_LDFLAGS PKG_CONFIG_PATH PKG_CONFIG_SYSROOT_DIR PKG_CONFIG_LIBDIR'
	} >"$w/cross.env"
	log "prepared $os: ${#entries[@]} verified inputs, GOCACHE key $key"
	log "toolchains: $go_v | $gcc_v | $clang_v | $lld_v | s390x $s390x_lld_v"
	echo "$w/cross.env"
}

verify() {
	local os=$1 a b f line settings tags ref_tags="" ref_bin="" goversion needed lib maxv failures=0 v1 v2 nm_hits elf
	need go readelf perl
	fail() { log "FAIL $*"; failures=$((failures + 1)); }
	[[ ${OCM_CROSS_OS:-} == "$os" && ${GOCACHE:-} == "$WORKDIR/gocache/$os-"* ]] ||
		fail "not running in the environment written by 'prepare $os' (OCM_CROSS_OS=${OCM_CROSS_OS:-} GOCACHE=${GOCACHE:-})"
	goversion=$(go env GOVERSION)
	for a in "${GOARCHES[@]}"; do
		v2=$ROOT/bin/linux-$a/oc-mirror
		v1=$ROOT/v1/bin/linux-$a/oc-mirror
		for b in "$v2" "$v1"; do
			f=${b#"$ROOT"/}
			[[ -x $b ]] || { fail "$f: missing"; continue; }
			settings=$(go version -m "$b" 2>&1) || true
			line=$(first_line "$settings")
			[[ $line == *": $goversion"* ]] || fail "$f: built with ${line#*: }, expected $goversion"
			grep -qP '^\tbuild\tCGO_ENABLED=1$' <<<"$settings" || fail "$f: CGO_ENABLED is not 1"
			grep -qP '^\tbuild\tGOEXPERIMENT=.*\bstrictfipsruntime\b' <<<"$settings" || fail "$f: GOEXPERIMENT lacks strictfipsruntime"
			grep -qP "^\tbuild\tGOOS=linux$" <<<"$settings" || fail "$f: GOOS is not linux"
			grep -qP "^\tbuild\tGOARCH=$a$" <<<"$settings" || fail "$f: GOARCH is not $a"
			tags=$(grep -oP '^\tbuild\t-tags=\K.*' <<<"$settings") || true
			[[ ,$tags, == *,json1,* ]] || fail "$f: missing the Makefile build tags (json1) in '$tags'"
			[[ ,$tags, == *,no_openssl,* || ,$tags, == *,containers_image_openpgp,* ]] && fail "$f: forbidden build tag in '$tags'"
			# The first binary that carries the Makefile tags (normally the natively built amd64 v2) is the reference.
			if [[ -z $ref_bin && ,$tags, == *,json1,* ]]; then
				ref_tags=$tags; ref_bin=$f
			elif [[ -n $ref_bin && $tags != "$ref_tags" ]]; then
				fail "$f: tags '$tags' differ from $ref_bin '$ref_tags'"
			fi
			nm_hits=$(go tool nm "$b" 2>/dev/null | grep -oE ' (vendor/github\.com/golang-fips/openssl/v2\.dlopen|x_cgo_init)$' | sort -u | wc -l) || true
			[[ $nm_hits == 2 ]] || fail "$f: golang-fips/openssl dlopen or cgo init symbol missing"
			elf=$(readelf -hlW "$b" 2>&1) || true
			[[ $elf =~ Machine:\ +${MACHINE[$a]}$'\n' ]] || fail "$f: ELF machine is not ${MACHINE[$a]}"
			[[ $elf == *"[Requesting program interpreter: ${INTERP[$a]}]"* ]] || fail "$f: interpreter is not ${INTERP[$a]} (static or wrong target)"
			needed=$(readelf -dW "$b" 2>/dev/null | grep -oP '\(NEEDED\).*\[\K[^]]+') || true
			for lib in "${REQUIRED_NEEDED[@]}"; do grep -qxF "$lib" <<<"$needed" || fail "$f: does not link $lib"; done
			while IFS= read -r lib; do
				[[ -z $lib || ${ALLOWED_NEEDED[$os]} == *" $lib "* ]] || fail "$f: unexpected shared library $lib"
			done <<<"$needed"
			maxv=$(readelf -VW "$b" 2>/dev/null | grep -oP 'Name: GLIBC_\K[0-9.]+' | sort -uV | tail -1) || true
			if [[ -z $maxv ]]; then
				fail "$f: no GLIBC symbol versions (not dynamically linked against glibc)"
			elif [[ $(printf '%s\n%s\n' "$maxv" "${GLIBC_MAX[$os]}" | sort -V | tail -1) != "${GLIBC_MAX[$os]}" ]]; then
				fail "$f: requires GLIBC_$maxv, newer than ${GLIBC_MAX[$os]} for $os"
			fi
		done
		if [[ -x $v1 && -x $v2 ]]; then
			perl -e 'local $/; open(my $a, "<:raw", $ARGV[0]) or die; open(my $b, "<:raw", $ARGV[1]) or die;
				my ($outer, $inner) = (<$a>, <$b>); exit(index($outer, $inner) >= 0 ? 0 : 1)' "$v2" "$v1" ||
				fail "bin/linux-$a/oc-mirror does not embed v1/bin/linux-$a/oc-mirror"
		fi
	done
	((failures == 0)) || die "$failures verification failure(s) for $os"
	log "verified $os: v2 and v1 for ${GOARCHES[*]} are CGO_ENABLED=1, strictfipsruntime, $goversion, tags '$ref_tags'"
}

case "${1:-}" in
prepare) check_os "${2:-}"; prepare "$2" ;;
verify) check_os "${2:-}"; verify "$2" ;;
*) die "$USAGE" ;;
esac
