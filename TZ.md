# AltSerialPort — Specification

Nov 1, 2026 · @Ivan · Draft

## 1. Overview

AltSerialPort is a desktop terminal for serial ports, made for low-level
debugging of hardware: microcontroller boards, USB-UART adapters, modems,
GPS and industrial devices. It is part of the AltBins series: one binary
for Linux, Windows and macOS, no installer dependencies, no root rights.

It is a standalone project: all its code lives in this repository and
nothing is taken from the other programs of the series. The only
dependencies are the UI library `nui` and `golang.org/x/sys`.

Several copies may run at once: one window per port is the usual way to
watch two boards, or both sides of a link.

**Main idea.** The first screen has only what every session needs: pick a
port, pick a speed, connect, read and type. A new user understands it
in five seconds without documentation. Everything else (hex view,
timestamps, plotter, macros, logging, signal lines, automation) is one
click or one shortcut away, but never in the way.

**Goals**

1. Zero friction: plug a board in, the port appears, one click connects.
2. Survive the device: when the board resets or is re-flashed and the
   port disappears, AltSerialPort reconnects by itself and keeps the log.
3. Show bytes as they are: text, hex and mixed views of the same data,
   with exact timestamps and RX/TX direction.
4. Depth without clutter: power features live in menus, the command
   palette and per-device profiles.
5. Automation: a CLI and an MCP server, so scripts and AI agents can read
   and write the port of a real device.

## 2. Analogs

| Program | Platforms | Strong | Weak |
| --- | --- | --- | --- |
| PuTTY | Win, Linux | Known everywhere, good VT emulation | Serial is an afterthought, no hex, no reconnect |
| Tera Term | Win | Macros, logging, XMODEM | Windows only, dated UI |
| RealTerm | Win | Every byte format, signal lines, capture | Overloaded UI, Windows only |
| HTerm | Win, Linux | Mixed text/hex in/out, send sequences | Dense UI, closed source, no updates |
| CoolTerm | Win, macOS, Linux | Simple, cross-platform, reconnect | Limited views, no plotter, slow on big logs |
| Docklight | Win | Protocol sequences, triggers, answers | Commercial, complex |
| YAT | Win | Rich send features, highlighting | Windows only, .NET, busy UI |
| CuteCom / moserial | Linux | Simple GUI | Few features |
| minicom / picocom / tio | Linux, macOS | Fast, scriptable, tio reconnects | Terminal only, no hex/plot, key-combo UX |
| Arduino Serial Monitor / Plotter | All | Plotter, zero setup | Locked to the IDE, minimal |
| Serial Studio | All | Dashboards from data frames | Heavy, needs frame config |

*Based on knowledge of the products; check current versions before release.*

**What AltSerialPort takes**

- tio: auto-reconnect, timestamps, hex mode, plain CLI.
- HTerm / RealTerm: mixed text+hex, any baud rate, signal lines.
- Arduino Plotter: live chart of numbers from text lines, zero setup.
- Docklight: macros, triggers and auto-answers — but kept out of sight.
- CoolTerm: one cross-platform window that "just works".

**Gap we fill.** No current tool is cross-platform, minimal on the first
screen, fast on huge logs, reconnecting, and has text/hex/plot views of the
same stream. None has an MCP interface for AI agents.

## 3. Main Window

One window, four zones. Nothing else is visible by default.

```
┌───────────────────────────────────────────────────────────────────────┐
│ ☰ [ttyUSB0 · CH340 ▾][115200 ▾][8N1 ▾] [Connect] │ Text Hex Mix │ ⏱ ⏸ ● 🔍 🗑  │ toolbar
├───────────────────────────────────────────────────────────────────────┤
│ boot: v1.4.2                                                          │
│ wifi: connecting...                                                   │ terminal
│ wifi: ok 192.168.1.40                                                 │
│ status                                       (TX lines in the accent) │
│ temp=23.4 hum=41                                                      │
├───────────────────────────────────────────────────────────────────────┤
│ [ type a command and press Enter...            ] [Text ▾] [CRLF ▾] Send │ send line
├───────────────────────────────────────────────────────────────────────┤
│ ● Connected  ttyUSB0 115200 8N1   RX 12.4 KB  TX 86 B   DTR RTS │ CTS DSR DCD RI │ status
└───────────────────────────────────────────────────────────────────────┘
```

**Toolbar: the main functions only**

| Item | What it does |
| --- | --- |
| ☰ | The first button: settings, tools, panels, theme, language, always on top, shortcuts, about |
| Port | List of ports with their chip and description; a new port is marked |
| Baud | Editable combo: presets or any number |
| Frame | 8N1, 8E1, 8O1, 7E1, 7O1, 8N2…; "More…" opens the port settings |
| Connect | The main button, in the accent color; Disconnect while open; "Waiting…" while the device is gone |
| Text / Hex / Mixed | The view of the data |
| Timestamps | Off / on; the kind of time is in the context menu |
| Pause | Freezes the screen; the data keeps coming to the buffer and the log |
| Record | Logs to a file; red while recording |
| Find | The search bar over the terminal |
| Clear | Clears the screen |

**Context menus: everything else, where it belongs**

| Place | Items |
| --- | --- |
| Terminal | Copy, Copy as hex, Copy as C array, Select all, Find, Clear, Save as…; Timestamps ▸ (off, time, time with µs, since previous, since connect); Line breaks ▸ (auto, LF, CR, CRLF); Encoding ▸; Wrap lines; Show sent data; Local echo; Packet gap ▸ |
| Send line | Mode ▸ (text with escapes, hex); Line ending ▸; Append checksum ▸; Repeat every…; Send file…; Clear history |
| Port | Refresh, Port details (path, VID:PID, serial number, driver), Port settings… |
| Signal lines in the status bar | DTR, RTS on/off; Send BREAK; Reset board ▸ (DTR pulse, ESP32 boot, ESP32 reset) |
| Counters in the status bar | Reset counters |

**Rules of the minimal design**

1. A new user needs only the toolbar and the send line.
2. The toolbar has only what is used in every session; all the rest is in
   the context menu of the place it is about.
3. Every toolbar button has a tooltip with its name and shortcut.
4. Settings are per-device profiles, applied automatically; there is no
   big settings dialog for the port.
5. Defaults are right for 90% of cases: 115200 8N1, no flow control,
   automatic line breaks, UTF-8, timestamps off.
6. Typing in the terminal sends keys at once (a console of the device);
   the send line sends whole commands.

## 4. Functional Requirements

Priority: **M** — in 1.0, **S** — in 1.x, **C** — later.

### 4.1. Ports and connection

| ID | Requirement | P |
| --- | --- | --- |
| PORT-1 | List ports with friendly names: USB VID/PID, chip (FTDI, CH340, CP210x, PL2303, CDC ACM), serial number, manufacturer | M |
| PORT-2 | Hot-plug: list updates live; a newly plugged port is highlighted and can be selected by one click | M |
| PORT-3 | Auto-reconnect when the port disappears and returns (board reset, re-flash); the log keeps a separator line with the gap time | M |
| PORT-4 | Any baud rate, including non-standard (e.g. 74880, 250000, 2000000); presets for common ones | M |
| PORT-5 | Data bits 5–8, parity N/E/O/M/S, stop bits 1/1.5/2 | M |
| PORT-6 | Flow control: none, RTS/CTS, XON/XOFF | M |
| PORT-7 | Signal lines: show CTS/DSR/DCD/RI live, toggle DTR/RTS, send BREAK | M |
| PORT-8 | Reset helpers: one-click "reset board" sequences (Arduino DTR pulse, ESP32 RTS/DTR boot and reset) | S |
| PORT-9 | Auto-baud: detect the rate by trying presets and scoring the received text | S |
| PORT-10 | Opening a busy port shows which process holds it (where the OS allows) | S |
| PORT-11 | Linux: if access is denied, explain the `dialout`/`uucp` group and offer the exact command | M |
| PORT-12 | RS-485 mode (Linux ioctl, direction control via RTS) | C |
| PORT-13 | Network ports: raw TCP client/server and RFC 2217 | S |

### 4.2. Views

| ID | Requirement | P |
| --- | --- | --- |
| VIEW-1 | Text view: UTF-8 and other encodings (ASCII, CP1251, CP866, KOI8-R, Latin-1) | M |
| VIEW-2 | Hex view: offset, hex bytes, ASCII column; RX and TX in different colors | M |
| VIEW-3 | Mixed view: text with control and non-printable bytes shown as tokens (`<CR>`, `<LF>`, `<00>`, `<1B>`) | M |
| VIEW-4 | Switching view re-renders the whole buffer, not only new data | M |
| VIEW-5 | Timestamps per line or per received chunk: absolute (ms/µs), relative to previous, relative to connect | M |
| VIEW-6 | Packet split in hex/mixed view by idle gap (configurable, e.g. 5 ms) or by delimiter byte | M |
| VIEW-7 | ANSI/VT100 colors and basic cursor control in text view; option to show escapes raw | M |
| VIEW-8 | Local echo on/off; TX shown inline with RX in accent color | M |
| VIEW-9 | Highlight rules: text or regex → color (e.g. `ERROR` red, `WARN` yellow); presets for common log formats | S |
| VIEW-10 | Filter: show only lines matching a text/regex, without losing other lines | S |
| VIEW-11 | Search with match count and next/previous; works in all views | M |
| VIEW-12 | Buffer of 1 million lines by default (up to 10 million in the settings) without lag; old data is dropped by limit | M |
| VIEW-13 | Clear, pause (freeze screen while still receiving), copy as text / hex / C array | M |

### 4.3. Sending

| ID | Requirement | P |
| --- | --- | --- |
| SEND-1 | Send line with history (Up/Down), persistent per profile | M |
| SEND-2 | Line endings: none, CR, LF, CRLF | M |
| SEND-3 | Input modes: text with escapes (`\r \n \x1B \0`), hex (`01 03 00 00 00 0A`), decimal bytes | M |
| SEND-4 | Raw keyboard mode: each key is sent at once, like picocom; Ctrl+key sends control codes | M |
| SEND-5 | Append checksum to sent bytes: CRC-16/Modbus, CRC-8, CRC-32, XOR, SUM | S |
| SEND-6 | Macros: named buttons in a side panel and Shift+F1–F12 (F1, F3, F5, F6 are taken by help, find, connect and focus), each a text/hex sequence with delays | S |
| SEND-7 | Repeat send: every N ms, N times or until stopped | S |
| SEND-8 | Send file: raw bytes, or line by line with delay per line/char | M |
| SEND-9 | File transfer protocols: XMODEM (CRC/1K), YMODEM | C |
| SEND-10 | Paste guard: large paste asks before sending and can use a delay per line | M |

### 4.4. Depth tools

| ID | Requirement | P |
| --- | --- | --- |
| TOOL-1 | Plotter: numbers in text lines (`23.4`, `t=23.4 h=41`, CSV) become live chart series with names; no setup needed | S |
| TOOL-2 | Triggers: on text/regex/hex match — highlight, beep, notify, send an answer, pause, or stop logging | S |
| TOOL-3 | Two ports in one window: split view, or merged timeline for sniffing a link with two adapters (RX of A and RX of B) | S |
| TOOL-4 | Bridge: forward port A ↔ port B (or TCP) and show the traffic | C |
| TOOL-5 | Statistics: bytes/s, lines/s, errors (framing, parity, overrun where the OS reports them) | S |
| TOOL-6 | Loopback test: send a pattern, check echo, report lost/corrupted bytes and round-trip time | S |
| TOOL-7 | Response time: time from TX to first RX byte, shown on the TX line | S |

### 4.5. Logging and sessions

| ID | Requirement | P |
| --- | --- | --- |
| LOG-1 | Log to file: plain text, text with timestamps, or raw binary | M |
| LOG-2 | Auto-log option per profile, file name with port and date; size-based rotation | S |
| LOG-3 | Save buffer as text, hex dump or HTML with colors | M |
| LOG-4 | Open a saved log/binary file in the viewer for analysis (offline mode) | S |

### 4.6. Profiles

| ID | Requirement | P |
| --- | --- | --- |
| PROF-1 | Settings are remembered per device (VID/PID + serial number, else port name) and applied on connect | M |
| PROF-2 | Named profiles ("ESP32 debug", "Modbus RTU 9600 8E1") with export/import as a file | S |
| PROF-3 | Recent connections list in the port menu | M |

### 4.7. Automation

| ID | Requirement | P |
| --- | --- | --- |
| AUTO-1 | CLI: `altserialport list`, `altserialport COM5 -b 115200` (terminal in console), `--log`, `--hex`, `--timestamp` | M |
| AUTO-2 | CLI send/expect: `altserialport send COM5 "AT\r" --expect OK --timeout 2s` for scripts and CI | S |
| AUTO-3 | MCP server: tools to list ports, open, read with timeout, write, wait for pattern, toggle lines; lets an AI agent work with a real board | S |
| AUTO-4 | GUI and CLI/MCP may share one open port: the window shows what the agent sends and receives | C |

### 4.8. Series features

| ID | Requirement | P |
| --- | --- | --- |
| SER-1 | Light and dark themes, AltBins look | M |
| SER-2 | Always on top, keyboard shortcuts, tooltips with shortcuts | M |
| SER-3 | Languages: English and Russian first, the others of the series later | M |
| SER-4 | Install/Update/Uninstall links on Windows, `.deb`/`.rpm`/user install on Linux, signed `.dmg` on macOS | S |
| SER-5 | Settings in `~/.altbins/.altserialport/` | M |
| SER-6 | Several copies at once, each with its own port; a port is opened exclusively, so two copies never read one port | M |

## 5. Non-functional Requirements

- **Latency.** Received byte on screen in under 20 ms; no line buffering
  delay in raw mode.
- **Throughput.** 3 Mbaud continuous input without loss or UI freeze;
  rendering is decoupled from reading.
- **No data loss.** Reading thread never blocks on UI; log file is
  written from the reader, not from the screen.
- **Startup** under 300 ms; binary under 20 MB.
- **Stability.** Unplugging a device in any state never crashes or hangs
  the program.
- **Privacy.** No network access except an explicit update check.

## 6. Architecture and Technologies

- Go, UI on `nui` like the rest of the series.
- Own serial layer on `golang.org/x/sys`: termios/`TCSETS2` (any baud) on
  Linux, `IOSSIOSPEED` on macOS, Win32 COM API on Windows; port discovery
  via sysfs/udev, IOKit, SetupAPI. No cgo.
- Core packages without UI: `serial` (ports, lines, discovery), `buffer`
  (timestamped RX/TX chunks and the rows of each view), `session` (the
  open port, reconnect, logging), `proto` (escapes, hex, checksums), and
  later `mcp`, `cli`. The GUI (`forms`) is a thin layer.
- One event buffer is the source for all views, the log, the plotter and
  MCP; each event has direction, timestamp and bytes.

## 7. Stages

1. **MVP (0.1).** PORT-1…7, 11; VIEW-1…5, 8, 11, 13; SEND-1…4, 8, 10;
   LOG-1, 3; PROF-1, 3; SER-*. Usable daily tool.
2. **0.2.** Reconnect polish, highlight, filter, macros, repeat send,
   checksums, packet split, CLI.
3. **0.3.** Plotter, triggers, two ports, statistics, reset helpers.
4. **0.4.** MCP server, CLI send/expect, network ports.
5. **1.0.** Hardening, performance tests, docs, site page.

## 8. Testing

- Virtual port pairs (`socat`/pty on Linux and macOS, com0com on Windows)
  for automatic tests of all views, send modes and logging.
- Real hardware matrix: FTDI, CH340, CP2102, PL2303, CDC ACM (STM32,
  RP2040, ESP32-S3), on all three OS.
- Stress: 3 Mbaud for 1 hour, repeated unplug/plug during transfer.

## 9. Decisions

- Name: AltSerialPort.
- Several copies at once; no single-instance lock.
- No code from other projects: everything is written here.
- Main functions on the toolbar, the rest in context menus.

## 10. Implementation Status

Nov 1, 2026: every requirement of section 4 is implemented.

| Area | Where |
| --- | --- |
| Ports PORT-1…13 | `serial` (network ports, RS-485, the holder of a busy port), auto-baud in ☰ → Tools |
| Views VIEW-1…13 | `buffer`, `render`, the terminal; rules in the terminal menu → Highlight and triggers; the filter in the find bar |
| Sending SEND-1…10 | the send line and its menu, the macro panel, ☰ → Send file / Receive file (`proto/xmodem`) |
| Depth tools TOOL-1…7 | ☰ → Plot, Second port (one time line or side by side, bridge), Tools (statistics, loopback, baud detection); the response time after each sent row |
| Logging LOG-1…4 | record button, ☰ → Record each connection (rotation by size), Save as (text, HTML, hex dump, raw), ☰ → Open a log or a file, drag and drop |
| Profiles PROF-1…3 | per device; ☰ → Profiles (named, export, import) |
| Automation AUTO-1…4 | `cli`, `mcp`, `remote` (the window shares its port with them) |
| Series SER-1…6 | themes, shortcuts, en/ru; `install` and `scripts/` for the packages |

Not checked yet: real hardware of each kind, Windows and macOS (built and
vetted only), the signing of the macOS package (needs a certificate),
`.rpm` (no rpmbuild here). XMODEM was checked against lrzsz.

Measured on a desktop CPU with 1 000 000 rows in the buffer: append 1.4 µs a
row, a frame 2–9 ms, a search over all rows 0.1 s.

## 11. Open Questions

1. Is VT100 emulation needed beyond colors (full-screen apps like `top` on
   an embedded Linux console)? It would be written here from scratch.
2. Should the plotter be in 1.0 (strong differentiator) or 1.x?
