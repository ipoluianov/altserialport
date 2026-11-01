package forms

import (
	"github.com/ipoluianov/nui/ui"
	"github.com/ipoluianov/nui/ui/i18n"
)

// Strings are the texts of the interface in one language
type Strings struct {
	// Toolbar
	PortTip, BaudTip, FrameTip, ConnectTip         string
	Connect, Disconnect                            string
	ViewTextTip, ViewHexTip, ViewMixedTip          string
	TimestampsTip, PauseTip, RecordTip, FindTip    string
	ClearTip, MenuTip                              string
	NewPort, OtherPort                             string
	OtherPortTitle, OtherPortPrompt, OtherPortDesc string

	// Terminal
	HintTitle, Hint1, Hint2                      string
	HintNoPortsTitle, HintNoPorts1, HintNoPorts2 string
	HintOpenTitle, HintOpen1, HintOpen2          string
	HintWaitingTitle, HintWaiting1               string
	NewDataPill, PausedPill                      string

	// Terminal menu
	Copy, CopyHex, CopyC, PasteToDevice, SelectAll string
	Find, Clear, SaveAs                            string
	Timestamps                                     string
	TimeModes                                      []string
	LineBreaksMenu                                 string
	LineBreaks                                     []string
	EncodingMenu, PacketGap, HexWidth              string
	Off, Milliseconds, BytesPerRow                 string
	WrapLines, ShowSent, EchoKeys, BackspaceMenu   string

	// Send line
	SendHint, SendHintHex, SendModeTip, LineEndingTip, Send string
	Cut, Paste                                              string
	AppendChecksum, ChecksumOn                              string
	RepeatMenu, RepeatTitle, RepeatPrompt, RepeatBad        string
	RepeatEmpty, StopRepeat                                 string
	SendFile, FileSent, FileCancelled                       string
	EscapesHelp, EscapesHelpTitle, EscapesHelpText          string
	ClearHistory                                            string
	PasteTitle, PasteQuestion                               string

	// Find
	FindHint, FindPrev, FindNext, FindClose, FindNone, FindCount string
	FindRegex, FindFilter, FindBadRegex                          string

	// Port
	RefreshPorts, PortDetails, PortSettings         string
	ResetBoard, ResetDTR, ResetESPBoot, ResetESPRun string
	SendBreak, ResetCounters                        string
	BaudLabel, DataBitsLabel, ParityLabel           string
	StopBitsLabel, FlowLabel, LinesLabel, LinesHint string
	Parities                                        []string
	FlowNone                                        string
	BadBaud, BadBaudShort                           string
	PortAppeared                                    string

	// State and errors
	StateOpen, StateWaiting, StateClosed      string
	NoPortSelected, CannotOpen, PressConnect  string
	ErrBusy, ErrPermission, ErrNotFound       string
	ErrNotOpen, ErrQueueFull, PermissionLinux string
	NoteConnected, NoteDisconnected, NoteLost string
	NoteReconnected, NoteSettings             string

	// Log
	RecordTitle, LogText, LogRaw, LogSaved string

	// Main menu
	NewWindow, Theme, ThemeDark, ThemeLight, Language string
	FontSize, FontBigger, FontSmaller, FontReset      string
	AlwaysOnTop, Shortcuts, ShortcutsText             string
	About, AboutText, LicenseText                     string

	ColorNames           []string
	DetectBaud           string
	DetectBaudFound      string
	DetectBaudNone       string
	DetectBaudRunning    string
	FileBadDelay         string
	FileCharDelay        string
	FileLabel            string
	FileLineDelay        string
	FileMode             string
	FileModes            []string
	FileOpened           string
	FileTooBig           string
	FileWaitingReceiver  string
	FileWaitingSender    string
	Loopback             string
	LoopbackNothing      string
	LoopbackOK           string
	LoopbackResult       string
	LoopbackRunning      string
	LoopbackTime         string
	MacroAdd             string
	MacroBadWait         string
	MacroHelp            string
	MacroMode            string
	MacroName            string
	MacroRun             string
	MacroRunning         string
	MacroText            string
	MacroTitle           string
	MacrosHint           string
	MacrosTitle          string
	OpenFile             string
	OpenWhileConnected   string
	PlotClear            string
	PlotHint             string
	PlotTitle            string
	PlotWindowTip        string
	PlotWindows          []string
	Port2Bridge          string
	Port2BridgeNeedsBoth string
	Port2BridgeOn        string
	Port2BridgeTip       string
	Port2Hint            string
	Port2Merged          string
	Port2SendA           string
	Port2SendB           string
	Port2Shown           string
	Port2Split           string
	Port2TargetTip       string
	Port2Title           string
	Port2ViewTip         string
	ProfileBadFile       string
	ProfileDelete        string
	ProfileExport        string
	ProfileFiles         string
	ProfileImport        string
	ProfileName          string
	ProfileSaveAs        string
	ProfileSaved         string
	Profiles             string
	ReceiveFile          string
	RuleActions          string
	RuleAdd              string
	RuleAnswer           string
	RuleAnswerHint       string
	RuleBad              string
	RuleBeep             string
	RuleCase             string
	RuleColor            string
	RuleDown             string
	RuleEdit             string
	RuleEnabled          string
	RuleKind             string
	RuleKindHex          string
	RuleKindRegex        string
	RuleKindText         string
	RuleLogLevels        string
	RuleNotify           string
	RulePattern          string
	RulePause            string
	RuleRemove           string
	RuleStopLog          string
	RuleTitle            string
	RuleToggle           string
	RuleUp               string
	RulesAbout           string
	RulesEmpty           string
	RulesTitle           string
	Statistics           string
	StatPort             string
	StatUptime           string
	StatReconnects       string
	StatReceived         string
	StatSent             string
	StatRxRate           string
	StatPeak             string
	StatTxRate           string
	StatLines            string
	StatLoad             string
	StatFraming          string
	StatParity           string
	StatOverrun          string
	StatBreaks           string
	StatErrors           string
	StatNoErrors         string
	StatResponse         string
	MenuPlot             string
	MenuMacros           string
	MenuPort2            string
	MenuRules            string
	MenuResponseTime     string
	MenuAutoLog          string
	MenuTools            string
	MenuReceive          string
	MenuHexDump          string
	MenuHTML             string
	RS485Label           string
	InstallAsk           string
	InstallFailed        string
	UninstallAsk         string
	Uninstalled          string
	HeldBy               string
	InstallLink          string
	UpdateLink           string
	UninstallLink        string
	Installed            string
	AttachedTo           string
}

var english = Strings{
	PortTip:         "Port. A port that has just appeared is marked as new",
	BaudTip:         "Baud rate: choose one or type any number",
	FrameTip:        "Data bits, parity, stop bits. … opens all the port settings",
	ConnectTip:      "Open or close the port",
	Connect:         "Connect",
	Disconnect:      "Disconnect",
	ViewTextTip:     "Text: the lines as a terminal shows them, with ANSI colors",
	ViewHexTip:      "Hex: the bytes, a new row after a pause in the data",
	ViewMixedTip:    "Mixed: the text with every control byte shown as \\r, \\n, \\x00",
	TimestampsTip:   "Timestamps. The kind of the time is in the menu of the terminal",
	PauseTip:        "Pause the screen; the data keeps coming to the buffer and the log",
	RecordTip:       "Record to a file",
	FindTip:         "Find",
	ClearTip:        "Clear the screen",
	MenuTip:         "Menu",
	NewPort:         "new",
	OtherPort:       "Other port…",
	OtherPortTitle:  "Other port",
	OtherPortPrompt: "The path or the name of the port: /dev/pts/3, COM12, tcp://host:port, rfc2217://host:port, tcp-listen://:5000",
	OtherPortDesc:   "by path",

	HintTitle:        "Choose a port and press Connect",
	Hint1:            "The settings of each device are remembered",
	Hint2:            "Right-click anywhere for more",
	HintNoPortsTitle: "No serial ports found",
	HintNoPorts1:     "Plug in a device: it will appear in the list by itself",
	HintNoPorts2:     "A port that is not listed: Other port… in the list",
	HintOpenTitle:    "Connected to %s",
	HintOpen1:        "Waiting for data. Type here to send keys to the device",
	HintOpen2:        "or send a whole command from the line below",
	HintWaitingTitle: "The device is gone",
	HintWaiting1:     "It will be opened again as soon as it is back",
	NewDataPill:      "↓  New data",
	PausedPill:       "❚❚  Paused — click to continue",

	Copy:          "Copy",
	CopyHex:       "Copy as hex",
	CopyC:         "Copy as C array",
	PasteToDevice: "Paste to the device",
	SelectAll:     "Select all",
	Find:          "Find…",
	Clear:         "Clear",
	SaveAs:        "Save as…",
	Timestamps:    "Timestamps",
	TimeModes: []string{"Off", "Time", "Time with microseconds",
		"Since the previous row", "Since the connection"},
	LineBreaksMenu: "Line breaks",
	LineBreaks:     []string{"Auto (CR, LF, CRLF)", "LF", "CR", "CRLF", "None"},
	EncodingMenu:   "Encoding",
	PacketGap:      "New row after a pause",
	HexWidth:       "Hex row width",
	Off:            "Off",
	Milliseconds:   "%d ms",
	BytesPerRow:    "%d bytes",
	WrapLines:      "Wrap long lines",
	ShowSent:       "Show sent data",
	EchoKeys:       "Show typed keys (local echo)",
	BackspaceMenu:  "Backspace sends",

	SendHint:         "Type a command and press Enter     \\r \\n \\x1B escapes work",
	SendHintHex:      "Hex bytes: 01 03 00 00 00 0A",
	SendModeTip:      "Text with escapes, or hex bytes",
	LineEndingTip:    "Line ending added to each command; Enter in the terminal sends it too",
	Send:             "Send",
	Cut:              "Cut",
	Paste:            "Paste",
	AppendChecksum:   "Append checksum",
	ChecksumOn:       "%s is added to each command",
	RepeatMenu:       "Repeat every…",
	RepeatTitle:      "Repeat",
	RepeatPrompt:     "Send the command every, ms:",
	RepeatBad:        "At least 10 ms",
	RepeatEmpty:      "Type the command to repeat first",
	StopRepeat:       "Stop",
	SendFile:         "Send file…",
	FileSent:         "%s sent: %s",
	FileCancelled:    "Sending stopped after %s",
	EscapesHelp:      "Escapes…",
	EscapesHelpTitle: "Escapes of the send line",
	EscapesHelpText: "In the text mode:\n\n" +
		"\\r   CR (0x0D)        \\n   LF (0x0A)\n" +
		"\\t   TAB              \\0   NUL\n" +
		"\\e   ESC (0x1B)       \\\\   backslash\n" +
		"\\xHH  any byte, \\x02\n" +
		"\\uHHHH  a Unicode character\n\n" +
		"In the hex mode: 01 03 0A, 0x01,0x03 or 01030A.\n\n" +
		"A line copied from the Mixed view can be sent back as it is.",
	ClearHistory:  "Clear history",
	PasteTitle:    "Paste",
	PasteQuestion: "Send %s (%d lines) to the device?",

	FindHint:     "Find",
	FindPrev:     "Previous",
	FindNext:     "Next",
	FindClose:    "Close",
	FindNone:     "Not found",
	FindCount:    "%d of %d",
	FindRegex:    "Regular expression",
	FindFilter:   "Show only the rows that match",
	FindBadRegex: "Bad expression",

	RefreshPorts:  "Refresh",
	PortDetails:   "Port details",
	PortSettings:  "Port settings…",
	ResetBoard:    "Reset board",
	ResetDTR:      "DTR pulse (Arduino)",
	ResetESPBoot:  "ESP32: boot mode (download)",
	ResetESPRun:   "ESP32: reset and run",
	SendBreak:     "Send BREAK",
	ResetCounters: "Reset counters",
	BaudLabel:     "Baud rate:",
	DataBitsLabel: "Data bits:",
	ParityLabel:   "Parity:",
	StopBitsLabel: "Stop bits:",
	FlowLabel:     "Flow control:",
	LinesLabel:    "Lines:",
	LinesHint:     "DTR and RTS are set when the port opens; the status bar switches them at any time.",
	Parities:      []string{"None", "Even", "Odd", "Mark", "Space"},
	FlowNone:      "None",
	BadBaud:       "Not a baud rate: %q",
	BadBaudShort:  "Type the baud rate as a number",
	PortAppeared:  "New port: %s",

	StateOpen:        "Connected",
	StateWaiting:     "Waiting for the device",
	StateClosed:      "Not connected",
	NoPortSelected:   "Choose a port first.",
	CannotOpen:       "Cannot open %s.",
	PressConnect:     "press Connect (F5)",
	ErrBusy:          "The port is busy: another program has it open.",
	ErrPermission:    "Access denied.",
	ErrNotFound:      "The port is not there any more.",
	ErrNotOpen:       "The port is not open",
	ErrQueueFull:     "The port doesn't take the data: the flow control holds it back",
	PermissionLinux:  "On Linux the serial ports belong to the group dialout. Add yourself to it, then log out and in again:",
	NoteConnected:    "Connected to %s at %d %s",
	NoteDisconnected: "Disconnected",
	NoteLost:         "The device is gone. Waiting for it to come back…",
	NoteReconnected:  "Reconnected to %s after %.1f s",
	NoteSettings:     "Settings: %d %s",

	RecordTitle: "Record to a file",
	LogText:     "Text log",
	LogRaw:      "Raw received bytes",
	LogSaved:    "Saved: %s",

	NewWindow:   "New window",
	Theme:       "Theme",
	ThemeDark:   "Dark",
	ThemeLight:  "Light",
	Language:    "Language",
	FontSize:    "Font size",
	FontBigger:  "Bigger",
	FontSmaller: "Smaller",
	FontReset:   "Default",
	AlwaysOnTop: "Always on top",
	Shortcuts:   "Keyboard shortcuts",
	ShortcutsText: "F5                 Connect / disconnect\n" +
		"Ctrl+1, 2, 3       Text, Hex, Mixed view\n" +
		"Ctrl+Shift+T       Timestamps\n" +
		"Ctrl+Shift+P       Pause\n" +
		"Ctrl+Shift+R       Record to a file\n" +
		"Ctrl+Shift+L       Clear\n" +
		"Ctrl+F, F3         Find, find next\n" +
		"F6                 Terminal ↔ send line\n" +
		"Ctrl+= / − / 0     Font size\n" +
		"Ctrl+Shift+G / M   Plot, macros\n" +
		"Shift+F1…F12       Run a macro\n\n" +
		"In the terminal the keys go to the device:\n" +
		"Ctrl+A…Z send control codes, the arrows send VT100 sequences.\n" +
		"Ctrl+C copies when text is selected, else sends 0x03.\n" +
		"Ctrl+Shift+C       Copy\n" +
		"Ctrl+Shift+V       Paste to the device\n" +
		"Shift+PgUp/PgDn    Scroll\n" +
		"Ctrl+Home/End      Start / end of the data",
	About:                "About",
	AboutText:            "A terminal for serial ports: simple outside, deep inside.",
	LicenseText:          "MIT License",
	ColorNames:           []string{"None", "Red", "Orange", "Yellow", "Green", "Cyan", "Blue", "Magenta", "Gray"},
	DetectBaud:           "Detect baud rate",
	DetectBaudFound:      "Baud rate found: %d",
	DetectBaudNone:       "No speed gave readable text. The device must be sending something while the speed is detected.",
	DetectBaudRunning:    "Trying the speeds on the data of the device…",
	FileBadDelay:         "Type the delays as numbers of milliseconds",
	FileCharDelay:        "Delay after each byte, ms:",
	FileLabel:            "File:",
	FileLineDelay:        "Delay after each line, ms:",
	FileMode:             "Send:",
	FileModes:            []string{"As it is", "Line by line", "XMODEM", "XMODEM-1K", "YMODEM"},
	FileOpened:           "File %s, %s",
	FileTooBig:           "The file is too large: %s",
	FileWaitingReceiver:  "Waiting for the receiver to start…",
	FileWaitingSender:    "Waiting for the device to send…",
	Loopback:             "Loopback test",
	LoopbackNothing:      "Nothing came back. Join TX and RX of the adapter, or use a device that echoes.",
	LoopbackOK:           "The line is clean: every byte came back as it was sent.",
	LoopbackResult:       "Sent: %d bytes\nReceived: %d bytes\nLost: %d bytes\nCorrupted: %d bytes",
	LoopbackRunning:      "Sending packets and checking the echo…",
	LoopbackTime:         "Round trip: min %s, avg %s, max %s",
	MacroAdd:             "Add a macro",
	MacroBadWait:         "Line %d: write #wait and milliseconds, e.g. #wait 100",
	MacroHelp:            "Each line is sent with the line ending of the send line. #wait 100 waits 100 ms; # starts a comment.",
	MacroMode:            "Mode:",
	MacroName:            "Name:",
	MacroRun:             "Run",
	MacroRunning:         "Macro: %s",
	MacroText:            "Lines:",
	MacroTitle:           "Macro",
	MacrosHint:           "Shift+F1…F12 run the first twelve. Right-click a macro to edit it.",
	MacrosTitle:          "Macros",
	OpenFile:             "Open a log or a file…",
	OpenWhileConnected:   "Disconnect the port first: the file is shown in its place.",
	PlotClear:            "Clear the chart",
	PlotHint:             "The numbers of the received lines appear here: 23.4, t=23.4 h=41, temp:23.4",
	PlotTitle:            "Plot",
	PlotWindowTip:        "The time shown",
	PlotWindows:          []string{"10 s", "1 min", "10 min", "1 h"},
	Port2Bridge:          "Bridge A ⇄ B",
	Port2BridgeNeedsBoth: "Connect both ports for the bridge",
	Port2BridgeOn:        "The bridge passes the data between A and B",
	Port2BridgeTip:       "What each port receives is sent to the other one; both directions are shown",
	Port2Hint:            "Choose port B above and press Connect",
	Port2Merged:          "Together",
	Port2SendA:           "→ A",
	Port2SendB:           "→ B",
	Port2Shown:           "Second port: its data are shown in orange",
	Port2Split:           "Side by side",
	Port2TargetTip:       "Where the send line sends: port A or B",
	Port2Title:           "Port B",
	Port2ViewTip:         "The data of B in the time line of A, or in a terminal of its own",
	ProfileBadFile:       "This is not a profile of AltSerialPort",
	ProfileDelete:        "Delete",
	ProfileExport:        "Export to a file…",
	ProfileFiles:         "Profiles",
	ProfileImport:        "Import from a file…",
	ProfileName:          "The name of the profile:",
	ProfileSaveAs:        "Save current as…",
	ProfileSaved:         "Profile: %s",
	Profiles:             "Profiles",
	ReceiveFile:          "Receive file",
	RuleActions:          "When found:",
	RuleAdd:              "Add…",
	RuleAnswer:           "Answer:",
	RuleAnswerHint:       "Sent to the device, e.g. y\\r",
	RuleBad:              "Check the pattern: it is empty or not valid",
	RuleBeep:             "Beep",
	RuleCase:             "Case-sensitive",
	RuleColor:            "Line color:",
	RuleDown:             "Down",
	RuleEdit:             "Edit…",
	RuleEnabled:          "Enabled",
	RuleKind:             "Pattern is:",
	RuleKindHex:          "Hex bytes",
	RuleKindRegex:        "Regular expression",
	RuleKindText:         "Text",
	RuleLogLevels:        "Add log levels",
	RuleNotify:           "Notify",
	RulePattern:          "Pattern:",
	RulePause:            "Pause",
	RuleRemove:           "Remove",
	RuleStopLog:          "Stop the log",
	RuleTitle:            "Rule",
	RuleToggle:           "On / off",
	RuleUp:               "Up",
	RulesAbout:           "A rule colors the lines that have a text, and can beep, notify, answer the device, pause the screen or stop the log.",
	RulesEmpty:           "No rules",
	RulesTitle:           "Highlight and triggers",
	Statistics:           "Statistics",
	StatPort:             "Port",
	StatUptime:           "Connected for",
	StatReconnects:       "Reconnections",
	StatReceived:         "Received",
	StatSent:             "Sent",
	StatRxRate:           "Receive speed",
	StatPeak:             "peak",
	StatTxRate:           "Send speed",
	StatLines:            "Lines a second",
	StatLoad:             "Line load",
	StatFraming:          "Framing errors",
	StatParity:           "Parity errors",
	StatOverrun:          "Overruns",
	StatBreaks:           "Breaks",
	StatErrors:           "Line errors",
	StatNoErrors:         "not reported by this driver",
	StatResponse:         "Response min / avg / max",
	MenuPlot:             "Plot",
	MenuMacros:           "Macros",
	MenuPort2:            "Second port",
	MenuRules:            "Highlight and triggers…",
	MenuResponseTime:     "Show response time",
	MenuAutoLog:          "Record each connection of this device",
	MenuTools:            "Tools",
	MenuReceive:          "Receive file",
	MenuHexDump:          "Hex dump",
	MenuHTML:             "HTML with colors",
	RS485Label:           "RS-485 mode (Linux)",
	HeldBy:               "It is open in: %s",
	InstallLink:          "Install",
	UpdateLink:           "Update",
	UninstallLink:        "Uninstall",
	Installed:            "AltSerialPort is installed",
	InstallAsk:           "AltSerialPort will be copied to %s and added to the Start menu, the desktop and the list of installed apps. If it is already installed there, it is updated. The settings and the profiles stay as they are.\n\nAltSerialPort will then restart from there.",
	InstallFailed:        "AltSerialPort could not be installed:\n\n%s",
	UninstallAsk:         "Remove AltSerialPort from this computer?\n\nThe settings, the profiles and the logs stay in %s",
	Uninstalled:          "AltSerialPort has been removed.",
	AttachedTo:           "Attached by %s",
}

var russian = Strings{
	PortTip:         "Порт. Только что появившийся порт помечен как новый",
	BaudTip:         "Скорость: выберите или введите любое число",
	FrameTip:        "Биты данных, чётность, стоп-биты. … открывает все настройки порта",
	ConnectTip:      "Открыть или закрыть порт",
	Connect:         "Подключить",
	Disconnect:      "Отключить",
	ViewTextTip:     "Текст: строки как в терминале, с цветами ANSI",
	ViewHexTip:      "Hex: байты, новая строка после паузы в данных",
	ViewMixedTip:    "Смешанный: текст, управляющие байты видны как \\r, \\n, \\x00",
	TimestampsTip:   "Метки времени. Вид времени — в меню терминала",
	PauseTip:        "Пауза экрана; данные продолжают поступать в буфер и лог",
	RecordTip:       "Запись в файл",
	FindTip:         "Поиск",
	ClearTip:        "Очистить экран",
	MenuTip:         "Меню",
	NewPort:         "новый",
	OtherPort:       "Другой порт…",
	OtherPortTitle:  "Другой порт",
	OtherPortPrompt: "Путь или имя порта: /dev/pts/3, COM12, tcp://хост:порт, rfc2217://хост:порт, tcp-listen://:5000",
	OtherPortDesc:   "по пути",

	HintTitle:        "Выберите порт и нажмите «Подключить»",
	Hint1:            "Настройки каждого устройства запоминаются",
	Hint2:            "Правый клик — дополнительные функции",
	HintNoPortsTitle: "Последовательные порты не найдены",
	HintNoPorts1:     "Подключите устройство — оно само появится в списке",
	HintNoPorts2:     "Порт не в списке: «Другой порт…» в списке портов",
	HintOpenTitle:    "Подключено к %s",
	HintOpen1:        "Ждём данных. Печатайте здесь — клавиши уйдут в устройство",
	HintOpen2:        "или отправьте целую команду из строки ниже",
	HintWaitingTitle: "Устройство пропало",
	HintWaiting1:     "Порт откроется снова, как только оно вернётся",
	NewDataPill:      "↓  Новые данные",
	PausedPill:       "❚❚  Пауза — нажмите, чтобы продолжить",

	Copy:          "Копировать",
	CopyHex:       "Копировать как hex",
	CopyC:         "Копировать как массив C",
	PasteToDevice: "Вставить в устройство",
	SelectAll:     "Выделить всё",
	Find:          "Найти…",
	Clear:         "Очистить",
	SaveAs:        "Сохранить как…",
	Timestamps:    "Метки времени",
	TimeModes: []string{"Выключены", "Время", "Время с микросекундами",
		"От предыдущей строки", "От подключения"},
	LineBreaksMenu: "Переводы строк",
	LineBreaks:     []string{"Авто (CR, LF, CRLF)", "LF", "CR", "CRLF", "Нет"},
	EncodingMenu:   "Кодировка",
	PacketGap:      "Новая строка после паузы",
	HexWidth:       "Ширина строки hex",
	Off:            "Выключено",
	Milliseconds:   "%d мс",
	BytesPerRow:    "%d байт",
	WrapLines:      "Переносить длинные строки",
	ShowSent:       "Показывать отправленное",
	EchoKeys:       "Показывать нажатые клавиши (локальное эхо)",
	BackspaceMenu:  "Backspace отправляет",

	SendHint:         "Введите команду и нажмите Enter     работают \\r \\n \\x1B",
	SendHintHex:      "Байты в hex: 01 03 00 00 00 0A",
	SendModeTip:      "Текст с escape-последовательностями или байты в hex",
	LineEndingTip:    "Окончание строки после каждой команды; его же отправляет Enter в терминале",
	Send:             "Отправить",
	Cut:              "Вырезать",
	Paste:            "Вставить",
	AppendChecksum:   "Добавлять контрольную сумму",
	ChecksumOn:       "К каждой команде добавляется %s",
	RepeatMenu:       "Повторять каждые…",
	RepeatTitle:      "Повтор",
	RepeatPrompt:     "Отправлять команду каждые, мс:",
	RepeatBad:        "Не меньше 10 мс",
	RepeatEmpty:      "Сначала введите команду для повтора",
	StopRepeat:       "Стоп",
	SendFile:         "Отправить файл…",
	FileSent:         "%s отправлен: %s",
	FileCancelled:    "Отправка остановлена после %s",
	EscapesHelp:      "Escape-последовательности…",
	EscapesHelpTitle: "Escape-последовательности строки отправки",
	EscapesHelpText: "В текстовом режиме:\n\n" +
		"\\r   CR (0x0D)        \\n   LF (0x0A)\n" +
		"\\t   TAB              \\0   NUL\n" +
		"\\e   ESC (0x1B)       \\\\   обратная косая\n" +
		"\\xHH  любой байт, \\x02\n" +
		"\\uHHHH  символ Unicode\n\n" +
		"В режиме hex: 01 03 0A, 0x01,0x03 или 01030A.\n\n" +
		"Строку, скопированную из смешанного вида, можно отправить обратно как есть.",
	ClearHistory:  "Очистить историю",
	PasteTitle:    "Вставка",
	PasteQuestion: "Отправить %s (%d строк) в устройство?",

	FindHint:     "Найти",
	FindPrev:     "Предыдущее",
	FindNext:     "Следующее",
	FindClose:    "Закрыть",
	FindNone:     "Не найдено",
	FindCount:    "%d из %d",
	FindRegex:    "Регулярное выражение",
	FindFilter:   "Показывать только совпавшие строки",
	FindBadRegex: "Ошибка в выражении",

	RefreshPorts:  "Обновить",
	PortDetails:   "Сведения о порте",
	PortSettings:  "Настройки порта…",
	ResetBoard:    "Сброс платы",
	ResetDTR:      "Импульс DTR (Arduino)",
	ResetESPBoot:  "ESP32: режим загрузчика",
	ResetESPRun:   "ESP32: сброс и запуск",
	SendBreak:     "Отправить BREAK",
	ResetCounters: "Обнулить счётчики",
	BaudLabel:     "Скорость:",
	DataBitsLabel: "Биты данных:",
	ParityLabel:   "Чётность:",
	StopBitsLabel: "Стоп-биты:",
	FlowLabel:     "Управление потоком:",
	LinesLabel:    "Линии:",
	LinesHint:     "DTR и RTS выставляются при открытии порта; в строке состояния их можно переключить в любой момент.",
	Parities:      []string{"Нет", "Чёт", "Нечёт", "Маркер (1)", "Пробел (0)"},
	FlowNone:      "Нет",
	BadBaud:       "Это не скорость: %q",
	BadBaudShort:  "Введите скорость числом",
	PortAppeared:  "Новый порт: %s",

	StateOpen:        "Подключено",
	StateWaiting:     "Ждём устройство",
	StateClosed:      "Не подключено",
	NoPortSelected:   "Сначала выберите порт.",
	CannotOpen:       "Не удаётся открыть %s.",
	PressConnect:     "нажмите «Подключить» (F5)",
	ErrBusy:          "Порт занят: его открыла другая программа.",
	ErrPermission:    "Нет доступа.",
	ErrNotFound:      "Порта больше нет.",
	ErrNotOpen:       "Порт не открыт",
	ErrQueueFull:     "Порт не принимает данные: их держит управление потоком",
	PermissionLinux:  "В Linux последовательные порты принадлежат группе dialout. Добавьте себя в неё и перезайдите в систему:",
	NoteConnected:    "Подключено к %s, %d %s",
	NoteDisconnected: "Отключено",
	NoteLost:         "Устройство пропало. Ждём, когда вернётся…",
	NoteReconnected:  "Снова подключено к %s через %.1f с",
	NoteSettings:     "Настройки: %d %s",

	RecordTitle: "Запись в файл",
	LogText:     "Текстовый лог",
	LogRaw:      "Принятые байты как есть",
	LogSaved:    "Сохранено: %s",

	NewWindow:   "Новое окно",
	Theme:       "Тема",
	ThemeDark:   "Тёмная",
	ThemeLight:  "Светлая",
	Language:    "Язык",
	FontSize:    "Размер шрифта",
	FontBigger:  "Крупнее",
	FontSmaller: "Мельче",
	FontReset:   "По умолчанию",
	AlwaysOnTop: "Поверх всех окон",
	Shortcuts:   "Горячие клавиши",
	ShortcutsText: "F5                 Подключить / отключить\n" +
		"Ctrl+1, 2, 3       Вид: текст, hex, смешанный\n" +
		"Ctrl+Shift+T       Метки времени\n" +
		"Ctrl+Shift+P       Пауза\n" +
		"Ctrl+Shift+R       Запись в файл\n" +
		"Ctrl+Shift+L       Очистить\n" +
		"Ctrl+F, F3         Поиск, следующее\n" +
		"F6                 Терминал ↔ строка отправки\n" +
		"Ctrl+= / − / 0     Размер шрифта\n" +
		"Ctrl+Shift+G / M   График, макросы\n" +
		"Shift+F1…F12       Запустить макрос\n\n" +
		"В терминале клавиши уходят в устройство:\n" +
		"Ctrl+A…Z — управляющие коды, стрелки — последовательности VT100.\n" +
		"Ctrl+C копирует, если есть выделение, иначе отправляет 0x03.\n" +
		"Ctrl+Shift+C       Копировать\n" +
		"Ctrl+Shift+V       Вставить в устройство\n" +
		"Shift+PgUp/PgDn    Прокрутка\n" +
		"Ctrl+Home/End      Начало / конец данных",
	About:                "О программе",
	AboutText:            "Терминал для последовательных портов: простой снаружи, мощный внутри.",
	LicenseText:          "Лицензия MIT",
	ColorNames:           []string{"Нет", "Красный", "Оранжевый", "Жёлтый", "Зелёный", "Голубой", "Синий", "Пурпурный", "Серый"},
	DetectBaud:           "Определить скорость",
	DetectBaudFound:      "Найдена скорость: %d",
	DetectBaudNone:       "Ни на одной скорости не получился читаемый текст. Пока скорость определяется, устройство должно что-то отправлять.",
	DetectBaudRunning:    "Перебираем скорости на данных устройства…",
	FileBadDelay:         "Введите задержки числом миллисекунд",
	FileCharDelay:        "Задержка после байта, мс:",
	FileLabel:            "Файл:",
	FileLineDelay:        "Задержка после строки, мс:",
	FileMode:             "Отправить:",
	FileModes:            []string{"Как есть", "Построчно", "XMODEM", "XMODEM-1K", "YMODEM"},
	FileOpened:           "Файл %s, %s",
	FileTooBig:           "Файл слишком большой: %s",
	FileWaitingReceiver:  "Ждём, когда приёмник начнёт…",
	FileWaitingSender:    "Ждём, когда устройство начнёт отправку…",
	Loopback:             "Петлевой тест",
	LoopbackNothing:      "Ничего не вернулось. Соедините TX и RX адаптера или используйте устройство с эхом.",
	LoopbackOK:           "Линия чистая: каждый байт вернулся таким, каким был отправлен.",
	LoopbackResult:       "Отправлено: %d байт\nПринято: %d байт\nПотеряно: %d байт\nИскажено: %d байт",
	LoopbackRunning:      "Отправляем пакеты и проверяем эхо…",
	LoopbackTime:         "Время туда и обратно: мин %s, сред %s, макс %s",
	MacroAdd:             "Добавить макрос",
	MacroBadWait:         "Строка %d: напишите #wait и миллисекунды, например #wait 100",
	MacroHelp:            "Каждая строка отправляется с окончанием строки отправки. #wait 100 ждёт 100 мс; # начинает комментарий.",
	MacroMode:            "Режим:",
	MacroName:            "Название:",
	MacroRun:             "Выполнить",
	MacroRunning:         "Макрос: %s",
	MacroText:            "Строки:",
	MacroTitle:           "Макрос",
	MacrosHint:           "Shift+F1…F12 запускают первые двенадцать. Правый клик по макросу — изменить.",
	MacrosTitle:          "Макросы",
	OpenFile:             "Открыть лог или файл…",
	OpenWhileConnected:   "Сначала отключите порт: файл показывается вместо его данных.",
	PlotClear:            "Очистить график",
	PlotHint:             "Здесь появятся числа из принятых строк: 23.4, t=23.4 h=41, temp:23.4",
	PlotTitle:            "График",
	PlotWindowTip:        "Показанное время",
	PlotWindows:          []string{"10 с", "1 мин", "10 мин", "1 ч"},
	Port2Bridge:          "Мост A ⇄ B",
	Port2BridgeNeedsBoth: "Для моста подключите оба порта",
	Port2BridgeOn:        "Мост передаёт данные между A и B",
	Port2BridgeTip:       "Принятое каждым портом отправляется в другой; видны оба направления",
	Port2Hint:            "Выберите порт B выше и нажмите «Подключить»",
	Port2Merged:          "Вместе",
	Port2SendA:           "→ A",
	Port2SendB:           "→ B",
	Port2Shown:           "Второй порт: его данные показаны оранжевым",
	Port2Split:           "Рядом",
	Port2TargetTip:       "Куда отправляет строка отправки: в порт A или B",
	Port2Title:           "Порт B",
	Port2ViewTip:         "Данные B в ленте A или в отдельном терминале",
	ProfileBadFile:       "Это не профиль AltSerialPort",
	ProfileDelete:        "Удалить",
	ProfileExport:        "Экспорт в файл…",
	ProfileFiles:         "Профили",
	ProfileImport:        "Импорт из файла…",
	ProfileName:          "Название профиля:",
	ProfileSaveAs:        "Сохранить текущий как…",
	ProfileSaved:         "Профиль: %s",
	Profiles:             "Профили",
	ReceiveFile:          "Принять файл",
	RuleActions:          "Когда найдено:",
	RuleAdd:              "Добавить…",
	RuleAnswer:           "Ответ:",
	RuleAnswerHint:       "Отправляется в устройство, например y\\r",
	RuleBad:              "Проверьте шаблон: он пустой или с ошибкой",
	RuleBeep:             "Звук",
	RuleCase:             "Учитывать регистр",
	RuleColor:            "Цвет строки:",
	RuleDown:             "Ниже",
	RuleEdit:             "Изменить…",
	RuleEnabled:          "Включено",
	RuleKind:             "Шаблон — это:",
	RuleKindHex:          "Байты в hex",
	RuleKindRegex:        "Регулярное выражение",
	RuleKindText:         "Текст",
	RuleLogLevels:        "Добавить уровни лога",
	RuleNotify:           "Уведомление",
	RulePattern:          "Шаблон:",
	RulePause:            "Пауза",
	RuleRemove:           "Удалить",
	RuleStopLog:          "Остановить лог",
	RuleTitle:            "Правило",
	RuleToggle:           "Вкл / выкл",
	RuleUp:               "Выше",
	RulesAbout:           "Правило окрашивает строки с заданным текстом и может подать звук, показать уведомление, ответить устройству, поставить паузу или остановить лог.",
	RulesEmpty:           "Правил нет",
	RulesTitle:           "Подсветка и триггеры",
	Statistics:           "Статистика",
	StatPort:             "Порт",
	StatUptime:           "Подключено",
	StatReconnects:       "Переподключений",
	StatReceived:         "Принято",
	StatSent:             "Отправлено",
	StatRxRate:           "Скорость приёма",
	StatPeak:             "пик",
	StatTxRate:           "Скорость отправки",
	StatLines:            "Строк в секунду",
	StatLoad:             "Загрузка линии",
	StatFraming:          "Ошибки кадра",
	StatParity:           "Ошибки чётности",
	StatOverrun:          "Переполнения",
	StatBreaks:           "BREAK",
	StatErrors:           "Ошибки линии",
	StatNoErrors:         "этот драйвер их не сообщает",
	StatResponse:         "Ответ мин / сред / макс",
	MenuPlot:             "График",
	MenuMacros:           "Макросы",
	MenuPort2:            "Второй порт",
	MenuRules:            "Подсветка и триггеры…",
	MenuResponseTime:     "Показывать время ответа",
	MenuAutoLog:          "Записывать каждое подключение этого устройства",
	MenuTools:            "Инструменты",
	MenuReceive:          "Принять файл",
	MenuHexDump:          "Hex-дамп",
	MenuHTML:             "HTML с цветами",
	RS485Label:           "Режим RS-485 (Linux)",
	HeldBy:               "Его открыла программа: %s",
	InstallLink:          "Установить",
	UpdateLink:           "Обновить",
	UninstallLink:        "Удалить",
	Installed:            "AltSerialPort установлен",
	InstallAsk:           "AltSerialPort будет скопирован в %s и добавлен в меню «Пуск», на рабочий стол и в список установленных приложений. Если он там уже установлен, он обновится. Настройки и профили останутся как есть.\n\nЗатем AltSerialPort перезапустится оттуда.",
	InstallFailed:        "Не удалось установить AltSerialPort:\n\n%s",
	UninstallAsk:         "Удалить AltSerialPort с этого компьютера?\n\nНастройки, профили и логи останутся в %s",
	Uninstalled:          "AltSerialPort удалён.",
	AttachedTo:           "Подключился %s",
}

var catalog = i18n.NewCatalog(english, map[string]Strings{"ru": russian})

// T returns the texts in the language of the interface
func T() *Strings { return catalog.Get(ui.Language()) }

// Languages are the languages of the interface
var Languages = []string{"en", "ru"}

// LanguageName returns the name of a language in that language
func LanguageName(lang string) string {
	switch lang {
	case "ru":
		return "Русский"
	}
	return "English"
}

// SetLanguage switches the interface to the language; "" is the system's
func SetLanguage(lang string) {
	if lang != "" {
		ui.SetLanguage(lang)
	}
}
