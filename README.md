# applemusic-tui — Apple Music in your terminal, on macOS **and** Linux

**English** · [Русский](README.ru.md)

![Go 1.26](https://img.shields.io/badge/Go-1.26-555?style=flat-square&logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/platforms-macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-555?style=flat-square)
![License: MIT](https://img.shields.io/badge/license-MIT-555?style=flat-square)

`amtui` is a terminal Apple Music player — a full TUI client with catalog
search, a live audio spectrum visualizer and synced lyrics, running on **Linux
as well as macOS**. It drives the official Apple Music web player inside a
hidden Chromium, so playback is the real thing. No DRM hacks: the browser plays
the audio legally, amtui just gives it a terminal face.

<p align="center"><img src="docs/media/demo.gif" alt="amtui — Apple Music in the terminal" width="800"></p>

## About this fork

This is a fork of [k1y0miiii/applemusic-tui](https://github.com/k1y0miiii/applemusic-tui)
focused on making amtui lighter on the machine it runs on, plus a real Windows
port. Measured on a Linux laptop (160×45 terminal, 60 s samples, % of one CPU
core; `bench/bench.sh` reproduces it):

| | amtui playing | amtui paused | Chrome playing | Chrome paused |
| --- | --- | --- | --- | --- |
| upstream | 35.1% | 35.8% | 4.4% | 0.6% |
| this fork | 10.7% | 3.0% | 3.6% | 0.2% → **0 after 10 min** |

What changed:

- **Cached artwork rendering.** Covers were re-rendered cell by cell on every
  frame (the single biggest CPU cost); they are now rendered once per size.
- **Idle frame pacing.** 30 fps only while music plays or something loads,
  5 fps otherwise; animations still advance in real time.
- **Faster panels.** Borders and joins measure each line once through a width
  cache; output is byte-identical to lipgloss (tested).
- **Events instead of hard polling.** MusicKit change events are pushed to
  amtui, so the UI reacts in milliseconds; the poll relaxes to 2 s playing /
  5 s paused and the 200-item queue is only re-sent when it changes.
- **Direct queue jumps.** Picking a song 40 places down calls
  `changeToMediaAtIndex` once instead of skipping through every track in
  between (19.5 s → 0.85 s measured).
- **Browser sleep.** After 10 minutes paused the hidden Chrome (~800 MB) is
  closed; any key or media key that needs the player brings it back with the
  same queue, track and position (~5–7 s).
- **Login fix.** The login browser is closed gracefully, so a fresh sign-in is
  actually saved instead of being lost on the restart.
- **Windows.** Real visualizer via WASAPI loopback capture, a PowerShell
  installer, and Windows-safe tests.
- **The panefx animations.** ASCII effects ported from
  [panefx](https://github.com/michael-slop) — flames, fire, waves, plasma,
  plasma-square (plasma folded into four mirrored quadrants that fill the panel),
  tunnel, starfield, cube — as visualizer modes
  (`v` / `V` cycle; `go run ./bench/fxdemo <name> 80 24 10` runs one alone
  against live audio). Each keeps its panefx look at reactivity 0 and
  answers the music above it; colours follow the theme.
- **The music drives the motion.** Every animation's clock runs at the
  music's pace: nearly still (4% speed) when paused or silent, easing to full
  speed as the song gets loud, so on and off are obvious at a glance.
- **Spectrum shaping** — the torus's trick for the effects: flames and fire
  (mirrored, bass in the centre), the tunnel wall (floor bass to ceiling
  treble) and waves (across the width) are shaped by all 32 bands at once.
- **Reactivity slider** (`[` / `]`). The analyzer's fixed dB scale is right
  for the bars (an honest EQ) but left loud masters sitting high and flat, so
  shapes barely moved (measured band wobble 0.02–0.12). Animations now use a
  per-band "change against its own recent average" signal (wobble ~0.3) and
  one slider, 0–100 %, scales how hard every animation answers.
- **Hidden colour controller** (`?` then `c`). Pick a preset — including the
  house `slop` palette — or tune any of the eight slots in HSL, live; enter
  keeps it (saved as `custom`), esc restores. Presets: apple, catppuccin,
  gruvbox, nord, mono, slop, tokyo-night, dracula, one-dark, rose-pine,
  kanagawa, everforest, solarized, monokai-pro, github-dark, ayu-mirage,
  night-owl, auto (follows the album art). `t` cycles them too.
  **On an Omarchy desktop with color.mesh**, the preset menu comes from the
  shared list `~/.config/color.mesh/presets.conf` instead (27 grouped themes,
  each read from its Omarchy `colors.toml`), plus apple, mono and auto — the
  same menu as the desktop picker; choosing here still only recolours amtui.
- **Lyrics panel collapses** when a track has none; the visualizer takes the
  column (`lyrics.collapse_when_missing = false` to keep it).
- **No 4 MB parser churn.** lipgloss borrows an ANSI parser with a 4 MB
  buffer from a pool that every GC empties; per-frame Width()/per-cell
  renders re-allocated it constantly. Hot paths now write colour codes
  directly (byte-identical, tested): CPU per frame roughly halved.
- **No log lines over the TUI.** chromedp's messages (e.g. Chrome 154's new
  `DOM.topLayerElementsUpdated`) go to `~/.config/amtui/amtui.log`.

## Why amtui

Terminal Apple Music clients are usually AppleScript remotes for the macOS
Music.app: Mac-only, and limited to steering an app that has to be running
anyway. amtui talks to Apple Music's **web** player through MusicKit instead,
which changes what is possible:

- **Linux, not just macOS.** The same Go binary runs on both. (Linux support is
  newer — see the note under [Requirements](#requirements).)
- **The full catalog, in the terminal.** Search songs, albums and playlists
  straight from MusicKit — no desktop app in the loop.
- **A real spectrum visualizer.** Actual system-audio PCM through an FFT, not a
  decorative animation.
- **Synced lyrics** from LRCLIB, with the album cover rendered next to them.
- **Your credentials stay out of the terminal.** You sign in inside a real
  browser window, so 2FA and captcha just work.

## Features

- **Full Apple Music catalog** — search songs, albums and playlists, plus your
  recently played, straight from MusicKit.
- **Queue** — jump to any track, append to queue, play next.
- **Recently played** — a cover grid under the queue: your last ten albums,
  playlists and singles as half-block artwork, arrow keys to pick, `↵` to play.
- **Live visualizer** — what is actually playing: system-audio PCM through a
  4096-sample Hann-window FFT into 32 bands (denser at low frequencies), 30 fps.
  CoreAudio process tap on macOS, PipeWire/PulseAudio monitor on Linux. Falls
  back to a clearly labeled simulated animation when live capture is
  unavailable. Three looks, cycled with `v`: Winamp-style **bars**, a spinning
  **torus** whose tube corrugates with the spectrum, and a wireframe **sphere**
  that turns slowly and pumps with the bass.
- **Synced lyrics** — timestamped lines from [LRCLIB](https://lrclib.net),
  no API keys, with the album cover rendered beside them in half-block color.
- **Last.fm scrobbling** — optional, off until you set it up. Counts time
  actually listened, so seeking to the end does not scrobble a track.
- **MPRIS on Linux** — media keys, panel widgets, lock screens and `playerctl`
  drive it like any desktop player. Appears automatically when there is a
  session bus.
- **Themes** — six built-in palettes cycled with `t`, overridable per color in
  a config file, plus an `auto` mode that pulls the accent from the artwork.
- **Transport** — play/pause, next/prev, seek, volume, shuffle, repeat, and a
  progress bar drawn as the waveform of what you have already heard.
- **Safe sign-in** — you log in inside a real browser window; your Apple ID
  credentials never touch the terminal.

## How it works

Apple Music has no public streaming API, and grabbing the stream would mean
breaking DRM — off the table. Instead, the browser plays the music legally and
amtui remote-controls it:

```
┌──────────────┐   chromedp (CDP)   ┌───────────────────────────┐
│  amtui (Go)  │ ◄────────────────► │ hidden Chromium           │
│  Bubble Tea  │                    │ music.apple.com, signed in│
└──────────────┘                    │ window.MusicKit (JS API)  │
                                    └───────────────────────────┘
```

One Go binary. It spawns a hidden Chromium with a persistent profile in
`~/.config/amtui/chrome` and talks to `window.MusicKit` in page context —
search, queue, play/pause, seek, now playing. No DOM scraping. Audio goes out
through the system mixer (the web player serves AAC 256; no lossless).

<a id="requirements"></a>

## Requirements

- An active **Apple Music subscription**
- **Go 1.26+** (to build from source)
- **Chrome or Chromium**
  - macOS: any recent Chrome/Chromium
  - Linux: a Widevine-capable browser — `google-chrome`, or Chromium with the
    Widevine plugin (e.g. `chromium-widevine` on the AUR)
- **macOS 14.2+** (the live visualizer uses a CoreAudio process tap),
  or **Linux** with `pipewire-pulse` (or plain PulseAudio) for live capture

> Linux support is **experimental** — designed for Arch/Ubuntu, currently less
> tested than macOS. On Hyprland the browser window is auto-hidden into a
> special workspace (Wayland forbids offscreen positioning); other Wayland
> compositors may leave the window visible for now.

> Windows: verified on Windows 11 — sign-in, full tracks, the WASAPI loopback
> visualizer, and the whole test suite. Install with `install.ps1` (below) and
> run it in Windows Terminal or Alacritty; the legacy console does not render
> the TUI correctly. The hidden browser stays out of the taskbar, Alt+Tab and
> tiling window managers (it is parked offscreen as a tool window), and it
> closes with amtui however amtui exits. Media keys work through Chrome's own
> Windows media integration while the browser is awake, and they wake a
> sleeping browser too: while it sleeps amtui holds play/pause, next and
> previous itself, and lets them go the moment the browser is back.

## Install

### Windows

```powershell
git clone https://github.com/michael-slop/applemusic-tui
cd applemusic-tui
powershell -ExecutionPolicy Bypass -File install.ps1
```

There is no separate Windows version to pull: this repository's `main` is the
Windows, Linux and macOS build at once, and Go picks the right files for each.

`install.ps1` builds with Go when it is installed (1.26 or newer), otherwise it
downloads the [latest release](https://github.com/michael-slop/applemusic-tui/releases/latest)
of this fork — no Go needed. It installs to `%LOCALAPPDATA%\Programs\amtui` and
adds that to your user PATH. You also need Google Chrome:
`winget install Google.Chrome`.

To update: `git pull`, then run `install.ps1` again — it can replace amtui
while it is still running.

If `amtui` is "not recognized" in a new terminal, that terminal inherited its
PATH from whatever started it before the install — a window manager such as
GlazeWM, or a launcher. Restart that program (or sign out and back in); until
then, run `& "$env:LOCALAPPDATA\Programs\amtui\amtui.exe"`.

### Prebuilt binaries

Grab the archive for your platform from the
[latest release](https://github.com/michael-slop/applemusic-tui/releases/latest) —
macOS (Apple Silicon / Intel), Linux (x86-64 / arm64) and Windows
(x86-64 / arm64):

```sh
tar -xzf amtui-*-linux-amd64.tar.gz
sudo install -m755 amtui-*/amtui /usr/local/bin/amtui
amtui --version
```

Every release ships a `SHA256SUMS` file; verify with `shasum -a 256 -c SHA256SUMS`.

macOS binaries are unsigned, so the first launch needs
`xattr -d com.apple.quarantine amtui` (or right-click → Open).

### From source

```sh
git clone https://github.com/michael-slop/applemusic-tui
cd applemusic-tui
./install.sh
```

The script builds from source and installs `amtui` (plus `applemusic` and
`applemusic-tui` aliases) into `~/.local/bin`, adding it to PATH for
zsh / bash / fish. `--prefix DIR` installs elsewhere, `--no-path` leaves your
shell config alone. If Chrome is missing, the installer offers to install it
(Homebrew on macOS, the official `.deb` or AUR on Linux).

On macOS the live visualizer needs the Xcode Command Line Tools
(`xcode-select --install`) — the installer checks for them.

Prefer doing it by hand? `go build -o amtui .` works too; a `CGO_ENABLED=0`
build runs the visualizer in simulated mode instead of capturing real audio.

Run the test suite with `make test`, or `make verify` for the full check.

> Building all release archives yourself: `make dist`. macOS is the only host
> that produces a complete set — the darwin builds need cgo for the CoreAudio
> visualizer, while the Linux and Windows targets are pure Go and
> cross-compile from anywhere.

## First run

1. Launch `./amtui` — a **visible** browser window opens on music.apple.com.
2. Sign in with your Apple ID. It is a normal browser, so 2FA and captcha
   just work.
3. Done — the window hides and the TUI takes over. The session persists in
   `~/.config/amtui/chrome`, so next launches go straight to the player.


## Keys

### Player

| Key | Action |
| --- | --- |
| `Space` | Play / pause |
| `n` / `p` | Next / previous track |
| `Tab` | Cycle focus: queue → recently played → transport |
| `j` `k` / `↓` `↑` | Queue: move selection · Recent: move a grid row · Transport: volume down / up |
| `Enter` | Play the selected queue track or recently-played cover |
| `←` / `→` | Recent: previous / next cover · Transport: seek −5 s / +5 s |
| `s` | Toggle shuffle |
| `r` | Cycle repeat mode |
| `?` | Show every key |
| `v` | Cycle visualizer: bars → torus → sphere |
| `t` | Cycle color theme |
| `R` | Reload the web player if it wedges |
| `/` | Open search |
| `q` / `Ctrl+C` | Quit |

### Mouse

Click a queue row or a cover to play it, click the progress bar to seek, and
scroll the wheel over the queue or the cover grid to move the selection.

### Search

| Key | Action |
| --- | --- |
| typing | Edit the query |
| `Enter` | In input: search (empty query — your library) · In list: play selection |
| `Tab` | Next tab: RECENT / SONGS / ALBUMS / PLAYLISTS |
| `↓` `↑` / `j` `k` | Move through results (`↓` from the input dives into the list) |
| `a` / `A` | Add to the end of the queue / play next |
| `Esc` | Close search |

<p align="center"><img src="docs/media/search.png" alt="Search overlay" width="800"></p>
<p align="center"><img src="docs/media/lyrics.png" alt="Queue, live visualizer and synced lyrics" width="800"></p>

## Visualizer

Press `v` to cycle three shapes; the choice is remembered in
`~/.config/amtui/vizmode`.

| Mode | What it draws |
| --- | --- |
| `bars` | 32-band spectrum with peak-hold markers. The default. |
| `torus` | A spinning donut. Its tube thickness at each angle comes from the band that owns that slice of the ring, so the shape corrugates with the spectrum. |
| `sphere` | A wireframe globe of latitude rings and meridians, shaded by depth. Frequency maps to latitude symmetrically, so the bass swells its waist. |

The two 3D shapes answer the beat differently. The torus answers with motion: a
bass kick speeds its spin up. The sphere answers with size — it turns at about a
third of that pace and instead swells and contracts on the beat, so the pump is
what you watch rather than the rotation.

Both read the beat as how far the low end rises above its own running average,
not as how loud it is. The bands are normalized over a 63 dB window, so a kick
riding 6 dB above the sustained bass moves the absolute number by a tenth and
across a track the low end sits high and nearly flat — driving anything from it
leaves the shape sitting still.

The bars do neither; they are the spectrum itself. Independent of the mode, the
panel borders pulse with the bass — `visualizer.pulse` turns that off.

Both 3D shapes are plain ASCII over the same 32 bands the bars use, so they cost
no extra audio work and follow the active theme's accent colors.

## MPRIS (Linux)

amtui publishes itself on D-Bus as `org.mpris.MediaPlayer2.amtui`, so the
desktop can drive it without the terminal being focused:

```sh
playerctl -p amtui play-pause
playerctl -p amtui metadata
```

Media keys, panel applets and lock screens pick it up on their own. Play,
pause, next, previous, seek and volume are wired; there is no track list and
nothing to raise, and both are declared as such. It appears whenever a session
bus is present and is silently skipped when there is not — a TTY, a container,
or macOS.

## Last.fm

Scrobbling is opt-in. Create an API account at
[last.fm/api/account/create](https://www.last.fm/api/account/create), put the
key and secret under `[lastfm]` in `~/.config/amtui/config.toml`, then run:

```sh
amtui lastfm-auth
```

It prints a link to approve, and saves the session key to
`~/.config/amtui/lastfm` (mode 600) — the key never has to go in the config
file. Nothing is sent until all three parts are present.

Tracks scrobble after half their length or four minutes, whichever comes first,
and never below 30 seconds — Last.fm's own rules. The clock counts time actually
listened rather than the playback position, so dragging the progress bar to the
end does not count as a play.

## Themes

Six built-in themes: `apple` (default), `catppuccin`, `gruvbox`, `nord`, `mono`
and `auto`. Press `t` to cycle them; the choice is remembered in
`~/.config/amtui/theme`.

`auto` takes its accent from the dominant color of the current track's album
art. Only the accent changes — background and text stay neutral, so no cover can
make the interface unreadable.

To pick a theme up front or override individual colors, copy
[`docs/config.example.toml`](docs/config.example.toml) to
`~/.config/amtui/config.toml`. The same file switches the peak markers, the bass
pulse, the waveform progress bar and the album art on or off.

## Configuration

| Environment variable | Effect |
| --- | --- |
| `AMTUI_CHROME` | Path to the Chrome/Chromium binary (overrides auto-detection) |
| `AMTUI_CONFIG_DIR` | Config directory (default `~/.config/amtui`) |
| `AMTUI_DEBUG` | Run the browser visibly, with verbose logging |
| `AMTUI_SLEEP_AFTER` | Browser sleep delay as a Go duration (`30s`, `5m`); overrides the config |
| `AMTUI_PPROF` | Serve Go's profiler on this address (e.g. `127.0.0.1:6060`) |

In `config.toml`:

```toml
[browser]
sleep_after_minutes = 10   # close the hidden browser after this long paused; 0 = never

[visualizer]
reactive = true            # animations use the per-band reactive signal (false = absolute levels)
reactivity = 50            # starting point for the [ ] slider, 0-100

[lyrics]
collapse_when_missing = true
```

## License

[MIT](LICENSE)
