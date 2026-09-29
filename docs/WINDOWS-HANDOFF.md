# amtui fork — Windows handoff (for a Claude Code session on Twin_Towers)

Written 2026-09-29 by the Claude session on SnatchedLenix7 (the Linux laptop).
Repo: https://github.com/michael-slop/applemusic-tui (branch `main`, same as
`power-tuning`). Upstream: https://github.com/k1y0miiii/applemusic-tui.

> **⚠ Never start Chrome from an SSH session on this box.** Each Chrome launch
> in a key-authenticated SSH session makes one failed interactive logon for
> `micha` (Security event 4625, status 0xc000006d/0xc000006a). Windows' lockout
> policy here is 10 failures in 10 minutes → `micha` is locked for 10 minutes,
> which also blocks every SSH login (OpenSSH/Admin: "get_user_token - unable to
> generate token", "ga_init, unable to resolve user micha"). This happened on
> 2026-09-29 14:07:51 when the engine tests (which launch Chrome ~10 times) were
> run over SSH, and was reproduced with a single headless launch. Run amtui and
> `go test ./engine/` only from an interactive desktop session (e.g. Windows
> Terminal on the machine).

## What amtui is

A Go terminal UI for Apple Music. It drives the official web player
(music.apple.com) inside a hidden Google Chrome through the Chrome DevTools
Protocol (chromedp). Only a Widevine-capable browser can play full tracks, so
**it must be Google Chrome** (or Edge via `AMTUI_CHROME`), not plain Chromium.
The user signs in with their Apple ID once in a visible Chrome window.

## What this fork changed (all on `main`)

| Commit topic | Files |
| --- | --- |
| Login fix: close the login browser gracefully so the session is saved | `engine/engine.go` |
| Artwork render cache | `artwork.go` |
| Idle frame pacing (30 fps playing, 5 fps idle, real-time catch-up) | `main.go` (`stepFrame`, `frameInterval`) |
| Cached-width panels/joins, byte-identical to lipgloss | `fastpanel.go`, `fastpanel_test.go` |
| MusicKit events pushed via CDP binding; poll 2 s/5 s; queue diffing | `engine/engine.go`, `engine/musickit.go`, `main.go` |
| Direct queue jump with `changeToMediaAtIndex` (fallback: stepping) | `engine/musickit.go` |
| Browser sleep after 10 min paused, snapshot/restore on wake | `engine/sleep.go`, `sleep.go`, `desktop.go` |
| Windows: WASAPI loopback visualizer, `install.ps1`, Windows-safe tests | `visualizer/source_windows.go`, `install.ps1`, `installer_test.go` |
| Opt-in profiler (`AMTUI_PPROF`) + benchmark harness | `pprof.go`, `bench/` |

Measured on Linux: amtui CPU 35%→11% playing, 36%→3% paused; a 40-song jump
19.5 s→0.85 s; Chrome (~800 MB) released after 10 minutes paused.

## Already verified on this Windows box (over SSH, 2026-09-29)

- `amtui.exe --version` runs (cross-compiled windows/amd64).
- WASAPI loopback capture: 48 kHz stereo; a 0.05-amplitude test tone read back
  as peak 0.0500 (`bench/wasapiprobe`).
- Every package's test binary passes on Windows, including the Chrome-driven
  engine tests (the POSIX `install.sh` tests are excluded on Windows).

## NOT verified yet — please do these

1. **Go toolchain.** The Scoop shim was broken: `go version` failed with
   "Shim: Could not create process". Fix with `scoop reset go` (or
   `scoop uninstall go; scoop install go`), or `winget install GoLang.Go`.
   Needs Go ≥ 1.26.
2. **Build + install:**
   ```powershell
   git clone https://github.com/michael-slop/applemusic-tui
   cd applemusic-tui
   $env:CGO_ENABLED = "0"; go test ./...
   powershell -ExecutionPolicy Bypass -File install.ps1
   ```
   `install.ps1` has never actually run on Windows (SSH dropped mid-test).
   Check it installs to `%LOCALAPPDATA%\Programs\amtui` and adds that to the
   user PATH. Chrome was present at `C:\Program Files\Google\Chrome\...`.
3. **First real run in Windows Terminal** (not over SSH — SSH sessions have no
   desktop or audio device): sign in when the Chrome window opens; confirm the
   login survives the "restarting browser" step; play something past 1:30 to
   prove full tracks (not previews) play.
4. **Visualizer:** press `v` to cycle; its title should say the source is live
   (`WASAPI`), not simulated.
5. **Hidden window:** upstream parks Chrome offscreen at -32000,-32000 and later
   minimizes it via CDP. Check whether it shows in the taskbar/Alt+Tab and
   whether playback survives track changes while minimized. If it stalls, a
   Windows `windowController` (see `engine/window_hyprland.go` for the pattern)
   could set `WS_EX_TOOLWINDOW` instead of minimizing.
6. **Media keys:** while the browser is awake, Chrome's own Windows media
   integration should handle play/pause/next. While amtui's browser is asleep,
   media keys do NOT wake it on Windows (MPRIS is Linux-only) — space in amtui
   does. Quick test of sleep: `$env:AMTUI_SLEEP_AFTER="30s"; amtui`, pause,
   wait, confirm Chrome exits, press space, confirm the same track resumes.
7. Optionally benchmark: `bench/bench.sh` is bash/tmux-based (Linux); on
   Windows compare CPU in Task Manager between upstream and this build.

## Useful facts

- Tests on a machine with a D-Bus session bus: one upstream test fails
  (`TestReadyStartsVisualizerOpenAlongsideInitialStateFetch`); irrelevant on
  Windows. On Linux run with `DBUS_SESSION_BUS_ADDRESS=disabled:`.
- Config: `%USERPROFILE%\.config\amtui\config.toml`; `[browser]
  sleep_after_minutes = 10` (0 disables). Chrome profile lives in
  `%USERPROFILE%\.config\amtui\chrome`.
- Env: `AMTUI_CHROME` (browser path), `AMTUI_DEBUG` (visible browser),
  `AMTUI_SLEEP_AFTER` (e.g. `30s`), `AMTUI_PPROF` (e.g. `127.0.0.1:6060`).
- Commit identity used so far: name `phreak`, email m.ichaelslo.p.io@gmail.com.
