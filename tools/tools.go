package tools

type ToolDef struct {
	Name               string
	Description        string
	Args               []ToolArg
	Secure             bool
	BlocksContext      bool
	Execute            func(args map[string]string) string
	ExecuteWithContext func(args map[string]string, senderID string) string
}

type ToolArg struct {
	Name        string
	Description string
	Required    bool
}

var All = []*ToolDef{

	Exec,
	RunPython,

	ReadFile,
	WriteFile,
	AppendFile,
	ListDir,
	CreateDir,
	DeleteFile,
	MoveFile,
	SearchFiles,

	SaveFact,
	RecallFact,
	ListFacts,
	DeleteFact,
	UpdateNote,

	WebFetch,
	WebSearch,

	IMDBSearch,
	IMDBGetTitle,

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

	GitHubSearch,
	GitHubReadFile,

	ScheduleTask,
	CancelTask,
	ListTasks,

	FlightAirportSearch,
	FlightRouteSearch,
	FlightCountries,

	NavGeocode,
	NavRoute,
	NavSunshade,

	Datetime,
	Timer,
	Echo,

	Calculate,
	Random,

	TextProcess,

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

	TGSendFile,
	TGSendPhoto,
	TGSendMessage,
	TGSendMessageWithButtons,
	SetBotDp,
	TGDownload,
	TGForwardMsg,
	TGDeleteMsg,
	TGPinMsg,
	TGUnpinMsg,
	TGGetChatInfo,
	TGReact,
	TGGetReply,
	TGGetMembers,
	TGBroadcast,
	TGGetMessage,
	TGEditMessage,
	TGCreateInvite,
	TGGetProfilePhotos,

	StockPrice,

	Pomodoro,
	DailyDigest,
	CronStatus,

	PinterestSearch,
	PinterestGetPin,

	UnitConvert,
	TimezoneConvert,
	Translate,

	ColorInfo,

	NewsHeadlines,
	RedditFeed,
	YouTubeSearch,
	ReadEmail,
	SendEmail,
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
}
