# AltSerialPort

A terminal for serial ports, made for low-level debugging of hardware:
microcontroller boards, USB-UART adapters, modems, GPS and industrial
devices. Choose a port, press **Connect**, read and type. Everything else is
one right-click away. It runs on Linux, Windows and macOS as a single
binary and needs no root rights.

Website: https://altbins.pro/altserialport/

## Features

- **Ports.** Every port with its chip and description (`ttyUSB0 · CH340`,
  `COM5 · CP2102`). A device plugged in appears in the list by itself, marked
  as new, and is selected when no port is open. Any port not in the list
  (a pseudo terminal, a udev link) is opened by its path.
- **Survives the device.** When a board resets, is re-flashed or its cable is
  pulled, AltSerialPort waits and opens the port again when it is back,
  even under another name. The log keeps a note of the gap.
- **Any speed and frame.** Any baud rate (74880, 250000, 2000000…), 5–8
  data bits, none/even/odd/mark/space parity, 1/1.5/2 stop bits, RTS/CTS
  and XON/XOFF. The speed and the frame change on the open port.
- **Three views of the same data.** Text (with ANSI colors), Hex (rows of
  bytes, a new packet after a pause, offsets within long packets) and Mixed
  (text with every control byte shown as `\r`, `\n`, `\x00`). Switching the
  view lays out all the data again and keeps the place.
- **Timestamps** of the time of day, with microseconds, since the previous
  row or since the connection.
- **Typing in the terminal sends keys** at once, as in a console of the
  device: Ctrl+A…Z, arrows, Home/End as VT100 sequences. The line below
  sends whole commands, with history, `\r \n \xHH` escapes or hex bytes,
  a line ending, an optional checksum (CRC-16 Modbus, CCITT, XMODEM, CRC-8,
  CRC-32, XOR, Sum, LRC) and repeat by timer.
- **Lines.** DTR and RTS are switched by a click in the status bar; CTS,
  DSR, DCD, RI are shown live. BREAK, and reset sequences for Arduino and
  ESP32 boards.
- **Logging** to a text file with timestamps or to a raw binary file,
  written from the reader of the port, so nothing is lost while the window
  is busy. Save the screen as text, copy a selection as text, hex or a C
  array.
- **Big data.** A million rows by default: drawing, scrolling and search
  stay instant.
- **Per-device memory.** The speed, the view, the line ending and the
  command history are remembered for each device and applied when it is
  chosen.
- **Several windows.** Each copy of the program is a window for one port;
  a port is opened exclusively, so two windows never read one port.
- **Depth tools** (☰ menu):
  - **Plot**: numbers of the received lines (`23.4`, `t=23.4 h=41`,
    `temp:23.4`) become live chart series with no setup.
  - **Macros**: named commands in a side panel, Shift+F1…F12, several
    lines with `#wait 100` between them.
  - **Highlight and triggers**: a text, a regular expression or hex bytes
    colors its lines, beeps, notifies, answers the device, pauses the
    screen or stops the log. Log levels (ERROR, WARN…) come preset.
  - **Find with a filter**: show only the rows that match, by text or a
    regular expression.
  - **Second port**: the data of two adapters on one time line (sniffing
    both sides of a link) or side by side, and a **bridge** that passes the
    data between them, also to a TCP port.
  - **Statistics**: speed, lines a second, line load, framing/parity/overrun
    errors where the driver reports them, response times.
  - **Response time** after each sent command, **loopback test**, **baud
    rate detection** from the data of the device.
  - **File transfer**: as it is, line by line with delays, XMODEM,
    XMODEM-1K, YMODEM; receiving by XMODEM/YMODEM.
  - **Logs**: automatic recording of each connection of a device, a new
    file past a size, saving as text, HTML with colors, hex dump or raw
    bytes, opening a saved log or any file (also by dropping it on the
    window).
  - **Profiles**: save the settings under a name, export and import them.
- **Network ports**: `tcp://host:port`, `tcp-listen://:5000` and
  `rfc2217://host:port` (a serial port of a network server), typed in
  "Other port…". **RS-485 mode** on Linux. A busy port tells which program
  holds it.
- **Command line and AI agents**: `altserialport list`, a console terminal
  (`altserialport /dev/ttyUSB0 -b 115200`), `altserialport send … --expect
  OK` for scripts, and `altserialport mcp`, an MCP server through which an AI
  agent works with a real board. When a window has the port open, they
  attach to it and the window shows what they send and receive.
- Light and dark themes, English and Russian.

## The window

| Place | What is there |
| --- | --- |
| Toolbar | ☰ menu; port, baud, frame, Connect; Text / Hex / Mixed; timestamps, pause, record, find, clear |
| Terminal | The data. Right-click: copy, timestamps, line breaks, encoding, packet gap, hex width, wrap, sent data, echo |
| Send line | Command, Text/Hex, line ending, Send. Right-click: checksum, repeat, send file, escapes |
| Status bar | State, port, RX/TX counters, log, lines. Click DTR/RTS to switch; right-click: reset board, BREAK, counters |

## Keyboard

| Keys | Action |
| --- | --- |
| F5 | Connect / disconnect |
| Ctrl+1, 2, 3 | Text, Hex, Mixed view |
| Ctrl+Shift+T / P / R / L | Timestamps, pause, record, clear |
| Ctrl+F, F3 | Find, find next |
| F6 | Switch between the terminal and the send line |
| Ctrl+= / Ctrl+- / Ctrl+0 | Font size |
| Ctrl+Shift+G, Ctrl+Shift+M | Plot, macros |
| Shift+F1…F12 | Run a macro |
| Ctrl+Shift+C, Ctrl+Shift+V | Copy, paste to the device |
| Shift+PgUp / PgDn, Ctrl+Home / End | Scroll |

In the terminal Ctrl+C copies when text is selected and sends 0x03 otherwise.

## Command line

```sh
altserialport list
altserialport /dev/ttyUSB0 -b 115200 --timestamp --log session.log   # Ctrl+] quits
altserialport send COM5 "AT\r" --expect OK --timeout 2s
altserialport mcp      # an MCP server on stdin/stdout
```

An MCP client (Claude Code, Claude Desktop…) runs `altserialport mcp`; its
tools list the ports, open one, write, read, wait for a pattern, set the
lines and reset ESP32/Arduino boards.

## Building

```sh
go build -ldflags="-s -w"                                 # Linux, macOS
GOOS=windows go build -ldflags="-s -w -H=windowsgui"
```

A release binary is about 18 MB. `scripts/build.sh` builds all the systems;
`scripts/package-deb.sh`, `package-rpm.sh`, `package-dmg.sh` and
`linux-install.sh` make the packages and the user-level install. On Windows
the program installs itself: ☰ → Install.

No C compiler is needed. The only dependencies are the UI library
[nui](https://github.com/ipoluianov/nui) and `golang.org/x/sys`.

Settings are stored in `~/.altbins/.altserialport/`.

### Linux

The serial ports belong to the group `dialout` (`uucp` on Arch). If a port
cannot be opened, add yourself to it and log in again:

```sh
sudo usermod -aG dialout $USER
```

## Code

| Package | What it does |
| --- | --- |
| `serial` | Ports on Linux (termios2, any baud), macOS (IOSSIOSPEED), Windows (overlapped I/O); the list of ports with USB IDs |
| `buffer` | The received and sent chunks with their times, laid out into rows of each view |
| `render` | A row into screen cells: encodings, ANSI colors, escapes, hex dump |
| `proto` | Escapes, hex input, line endings, checksums |
| `session` | The open port: reader, writer, reconnection, log file |
| `config` | Settings and per-device profiles, shared safely by several copies |
| `forms` | The window |

Tests: `go test ./...` (the port and window tests use pseudo terminals on
Linux; the window tests run without a display).

## License

MIT
