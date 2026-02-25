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

	TGSendFile,
	TGSendMessage,
	TGDownload,
	TGGetChatInfo,
	TGForwardMsg,
	TGDeleteMsg,
	TGPinMsg,
	SetBotDp,
	TGReact,
	TGGetReply,

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

	TGSendMessageWithButtons,
	ReadDocument,
	ListDocuments,
	SummarizeDocument,

	InstagramToTG,
}
