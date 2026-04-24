package tools

import (
	"os"
	"strings"
)

type ToolDef struct {
	Name               string
	Description        string
	Args               []ToolArg
	Secure             bool
	BlocksContext      bool
	Sequential         bool
	Execute            func(args map[string]string) string
	ExecuteWithContext func(args map[string]string, senderID string) string
}

type ToolArg struct {
	Name        string
	Description string
	Required    bool
}

var All []*ToolDef

func init() {
	base := []*ToolDef{
		Exec,
		ExecChain,
		RunPython,

		DeepWork,

		ReadFile,
		WriteFile,
		EditFile,
		AppendFile,
		GrepFile,
		ListDir,
		CreateDir,
		DeleteFile,
		MoveFile,
		SearchFiles,

	KBAdd,
	KBSearch,
	KBList,
	KBDelete,

	WebFetch,
	WebSearch,
	TavilySearch,
	TavilyExtract,
	TavilyResearch,

	IMDBSearch,
	IMDBGetTitle,

	YouTubeTranscript,

	TVMazeSearch,
	TVMazeNextEpisode,

	PatBinCreate,
	PatBinGet,

	BrowserOpen,
	BrowserClick,
	BrowserType,
	BrowserGetText,
	BrowserEval,
	BrowserScreenshot,
	BrowserWait,
	BrowserSelect,
	BrowserScroll,
	BrowserTabs,
	BrowserCookies,
	BrowserFormFill,
	BrowserPDF,

	GitHubSearch,
	GitHubReadFile,

	ScheduleTask,
	CancelTask,
	PauseTask,
	ResumeTask,
	ListTasks,

	FlightAirportSearch,
	FlightRouteSearch,
	FlightCountries,

	NavGeocode,
	NavRoute,
	NavSunshade,

	Datetime,

	Calculate,

	Weather,
	IPLookup,
	DNSLookup,
	HTTPRequest,
	RSSFeed,

	Wikipedia,
	CurrencyConvert,
	HashText,
	EncodeDecode,
	RegexMatch,

	SystemInfo,
	ProcessList,
	KillProcess,
	ClipboardGet,
	ClipboardSet,
	UpdateClaw,
	RestartClaw,
	KillClaw,

	TGSendMessage,
	TGSendFile,
	TGSendPhoto,
	TGSendAlbum,
	TGSendLocation,
	TGSendMessageWithButtons,
	SetBotDp,
	TGDownload,
	TGGetFile,
	TGForwardMsg,
	TGDeleteMsg,
	TGPinMsg,
	TGUnpinMsg,
	TGGetChatInfo,
	TGReact,
	TGGetMembers,
	TGBroadcast,
	TGGetMessage,
	TGEditMessage,
	TGCreateInvite,
	TGGetProfilePhotos,
	TGBanUser,
	TGMuteUser,
	TGKickUser,
	TGPromoteAdmin,
	TGDemoteAdmin,

	WASendMessage,
	WASendFile,
	WAGetContacts,
	WAGetGroups,

	StockPrice,

	DailyDigest,
	CronStatus,

	PinterestSearch,
	PinterestGetPin,

	UnitConvert,
	TimezoneConvert,
	Translate,
	Humanize,

	MCPCall,
	MCPList,
	MCPAuth,
	MCPConfig,

	NewsHeadlines,
	RedditFeed,
	RedditThread,
	YouTubeSearch,
	CalendarListEvents,
	CalendarCreateEvent,
	CalendarDeleteEvent,
	CalendarUpdateEvent,
	TextToSpeech,

	TodoAdd,
	TodoList,
	TodoDone,
	TodoDelete,

	DownloadYtdlp,
	DownloadAria2c,
	ReadDocument,
	ListDocuments,
	SummarizeDocument,

	PDFCreate,
	PDFExtractText,
	PDFMerge,
	PDFSplit,
	PDFRotate,
	PDFInfo,
	LaTeXCreate,
	LaTeXEdit,
	LaTeXCompile,
	DocumentSearch,

	DocumentCompress,
	DocumentWatermark,
	MarkdownToPDF,
	ImageResize,
	ImageConvert,
	ImageCompress,
	VideoTrim,
	AudioExtract,
	VideoExtractFrames,

	QRCodeGenerate,
	URLShorten,
	UUIDGenerate,
	PasswordGenerate,
	JokeFetch,

	MonitorAdd,
	MonitorList,
	MonitorRemove,

	CodeReview,

	ScreenCapture,

	ToolCreate,
	ToolListCustom,
	ToolDeleteCustom,
	ToolRunCustom,

	MemoryExtract,
	MemoryRecall,
	MemoryForget,
	MemoryStats,
	}

	All = base
	if strings.TrimSpace(os.Getenv("MATON_API_KEY")) != "" {
		All = append(All, GmailListMessages, GmailGetMessage, GmailSendMessage, GmailModifyLabels)
	} else {
		All = append(All, ReadEmail, SendEmail)
	}
}
