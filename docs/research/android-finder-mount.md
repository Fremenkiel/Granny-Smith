# Android-as-Finder-Volume on macOS — Research Notes

Research conversation captured 2026-05-02. Goal: build a macOS Go app that exposes an Android device (read via ADB) as an external volume in Finder.

---

## 1. Initial question — mounting external devices from Go on macOS

### darwinkit
No disk-mount support. Wraps AppKit / Foundation / Metal / CoreData Objective-C frameworks. Mounting lives in the **DiskArbitration** framework (C API, CoreFoundation), outside darwinkit's autogen scope. Won't help.

### Real options for "mount an external disk"

**1. DiskArbitration via cgo** — proper API.
- `DASessionCreate`, `DADiskCreateFromBSDName`, `DADiskMount`, `DADiskUnmount`
- Async callbacks, disk-appeared/disappeared events, mount approval hooks
- No subprocess, no parsing, structured errors
- Cost: cgo, CoreFoundation runloop boilerplate
- No mature Go wrapper exists. Roll your own cgo shim.

**2. `diskutil` via `os/exec`** — pragmatic.
- `diskutil mount -mountPoint /path /dev/disk2s1`
- No sudo needed for removable media owned by console user
- Easy, ships with macOS

**3. macFUSE / FUSE-T** — only if you mean "expose a folder *as* a device". Different problem.

### `diskutil` security concerns

| Risk | Mitigation |
|------|-----------|
| Command injection | Use `exec.Command(name, args...)` not shell string. Never `sh -c`. Never interpolate user input into single arg string. |
| Path traversal in mountpoint | Validate mountpoint: absolute, inside allowed root, no `..`, resolve symlinks before passing |
| BSD name spoofing | Validate `/dev/diskNsM` format with regex; verify via `diskutil info -plist` before mount |
| TOCTOU on mountpoint dir | Create dir with `0700` immediately before mount, owned by intended uid |
| Privilege escalation | Don't run app as root. Removable mounts work as user. If sudo ever needed, gate behind explicit user prompt, never cache. |
| Output parsing brittle | Use `-plist` flag → parse with `howett.net/plist`. Never regex the human output. |
| Log leakage | Don't log full device paths/labels if sensitive |

### Recommendation
- **One-shot mount, trusted input** → `diskutil` + `-plist` parsing + `exec.Command` arg array. Fine.
- **Daemon, watching for inserts, programmatic UX** → cgo + DiskArbitration. Worth the boilerplate.

---

## 2. Refined goal — expose a folder *as* a device

No native bind mount on macOS. Three real paths:

### a) FUSE loopback (live folder view)

| Backend | Install | Perms | Notes |
|--------|---------|-------|-------|
| **FUSE-T** | brew, no kext, no reboot | user-space (NFS shim) | Easiest UX. Closed source but free. Some xattr/lock caveats. |
| **macFUSE** | pkg, kext load, SIP/Reduced Security on Apple Silicon, reboot, user approval in System Settings | kernel | Highest fidelity, full POSIX semantics. Hostile install on Apple Silicon. |

**Go libs:**
- `github.com/hanwen/go-fuse/v2` — has ready-made `fs/loopback` example. Works with macFUSE and FUSE-T.
- `bazil.org/fuse` — older, macFUSE only really.
- `github.com/jacobsa/fuse` — also viable.

Pick `go-fuse`.

### b) Disk image from folder (snapshot, not live)
```
hdiutil create -srcfolder /path/to/folder -volname Foo foo.dmg
hdiutil attach foo.dmg
```
Real volume, no third-party deps. **Snapshot at create time** — folder edits after don't reflect.

### c) Local NFS/SMB loopback
Run macOS built-in `nfsd` or SMB server with export at folder, then mount `localhost:/folder`. No third-party deps. Heavyweight, perms tricky. Roughly what FUSE-T does internally.

### Folder-as-device security concerns

| Risk | Mitigation |
|------|-----------|
| Symlink escape from source folder | Resolve & validate symlinks in loopback impl |
| `allow_other` exposes mount to all uids | Don't set unless required; default is owner-only |
| Mountpoint TOCTOU | Create mountpoint with `0700`, owned by app uid, immediately before mount |
| Unmount on crash leaves stale mount | Defer unmount; on startup check `mount` output and clean up |
| macFUSE kext = kernel attack surface | Prefer FUSE-T. If macFUSE required, pin known-good version. |
| FUSE-T's NFS shim leaks via loopback NFS | Bind to `127.0.0.1` only (FUSE-T does by default, verify) |
| Path injection if mount/source paths come from user | Validate absolute, canonicalize, confine to allowed root |
| Privilege | App runs as user. No sudo needed. Don't elevate. |
| Codesigning/notarization | macFUSE requires kext approval; sign + notarize your binary |
| Resource exhaustion | Cap concurrent ops in FUSE handler; honor cancellation |

---

## 3. Concrete goal — Android compatibility layer (ADB → Finder volume)

### ADB vs MTP — the foundational decision

| | MTP | ADB |
|--|-----|-----|
| User setup | Plug in, pick "File Transfer" on phone | Enable Developer Options → USB debugging → tap "Allow" prompt |
| Audience | Anyone | Developers / power users |
| Wire | USB only | USB + TCP (wifi adb) |
| Scope | Media-store paths only (`/sdcard`, etc.) | Anything `shell` user can read; with root, everything |
| Reliability | Flaky, locks device | Solid, persistent |

ADB makes sense if: you need shell paths beyond MTP, want wifi, want root, or target is devs. Otherwise MTP is the right protocol.

### Prior art — read before writing

- **`hanwen/go-mtpfs`** — Go, FUSE, mounts Android as Finder volume via MTP. Author also wrote go-fuse. Close to exactly this app, minus ADB. Either use as-is or fork transport layer.
- **`whoozle/android-file-transfer-linux`** — C++, FUSE, macOS build. MTP. Reference for protocol quirks.
- **MacDroid** — closed-source UX reference.
- **`zach-klippenstein/goadb`** — Go client speaking ADB wire protocol to local `adb` server on `:5037`. What to build on if going ADB.

### ADB-backed architecture

```
Finder ↔ FUSE (virtual FS, go-fuse) ↔ ADB transport (goadb) ↔ adb server :5037 ↔ device
```

Layers:
1. **FUSE layer** — `go-fuse` v2, implement `fs.NodeGetattrer/Lookuper/Readdirer/Opener/Reader/Writer/Creater/Unlinker/...`. Virtual FS, not loopback.
2. **ADB transport** — `goadb` for sync (`STAT/LIST/SEND/RECV`) and shell. Don't shell out to `adb` CLI per op — too slow, arg-quoting hell. Speak wire protocol.
3. **Hot-plug** — `host:track-devices` stream auto-mounts/unmounts as phones connect.
4. **Mount backend** — FUSE-T (no kext) preferred, macFUSE fallback.

### Performance landmines specific to ADB

- **ADB sync `RECV` has no offset.** Streams whole file from byte 0. Finder + Quick Look = random reads on 4 GB videos = full pull each time.
- Mitigation: per-file local cache. On first `open`, pull whole file to temp dir; serve `read()` from local copy. Evict on close + timeout. Same trick MTP fuse drivers use.
- **`STAT`/`LIST` slow** (~10–50 ms per call). Aggressive readdir + getattr cache (1–5 s TTL).
- **Writes**: buffer to local temp, `SEND` on `release()`. Show as "syncing" if possible.
- **Don't shell `adb pull` / `push` per op.** Stay in goadb wire protocol.

### ADB-specific security concerns

| Risk | Mitigation |
|------|-----------|
| adbkey grants full shell access to any device pairing it | Keep `~/.android/adbkey` 0600, document trust model |
| adb-over-TCP binds to `0.0.0.0` if user enabled wifi adb | Detect & warn; prefer USB unless user opts in |
| Shell injection if building `adb shell` strings from paths | Stay in sync protocol. If shelling, `exec.Command` arg array; never interpolate device paths into shell strings — Android shell is real `sh`. |
| Other macOS users seeing mount | Owner-only perms; don't set `allow_other` |
| Stale temp cache leaks device data on disk | Cache in user-only dir (`~/Library/Caches/yourapp`, 0700); wipe on unmount; consider encrypted temp (sparseimage) for sensitive use |
| Multiple ADB clients race (Android Studio also running) | Reuse local adb server; don't `adb kill-server` |
| Bundled `adb` binary version drift | Require user-installed platform-tools, or bundle + sign your own; don't auto-update from internet |
| Notarization | Sign + notarize; FUSE-T install requires user action, document it |

### Recommendation

1. **Validate the ADB-vs-MTP choice first.** If audience isn't devs, switch to MTP, fork `go-mtpfs`, ship in a week.
2. If ADB is correct: `go-fuse` + `goadb` + FUSE-T, with whole-file caching strategy. Read `go-mtpfs` source first — same FUSE shape, different transport.
3. Build read-only first. Writes double the bug surface (rename semantics, partial writes, atomicity).

### Open questions to resolve before planning

1. **Audience — devs only, or end users?** *Recommend: devs. If end users, pivot to MTP.*
2. **Read-only or read-write?** *Recommend: read-only v1, write in v2.*
3. **Single device or multi-device (each as separate Finder volume)?** *Recommend: multi, via `track-devices`.*
4. **USB only, or wifi adb too?** *Recommend: USB only v1.*
5. **Bundle `adb` binary or require platform-tools installed?** *Recommend: require. Avoids signing/update burden.*
6. **FUSE-T required, or also support macFUSE?** *Recommend: FUSE-T primary, macFUSE optional.*
7. **Target macOS versions + arch?** *Recommend: macOS 13+, universal binary.*
