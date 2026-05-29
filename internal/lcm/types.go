package lcm

// Summary kinds.
const (
	KindLeaf        = "leaf"
	KindCondensed   = "condensed"
	KindRepo        = "repo"
	KindArchiveStub = "archive_stub"
)

// CharsPerToken is the estimated characters per token for token estimation.
const CharsPerToken = 4

// LargeOutputThreshold is the token count above which tool output is stored in LCM.
const LargeOutputThreshold = 50000

// MaxCompactionRounds is the maximum number of compaction rounds before giving up.
const MaxCompactionRounds = 10

// MinMessagesToSummarize is the minimum number of messages required for summarization.
const MinMessagesToSummarize = 3

// FallbackMaxChars is the maximum character count for deterministic fallback summaries.
const FallbackMaxChars = 2048

// SummaryIDPrefix and FileIDPrefix are the prefixes for LCM identifiers.
const (
	SummaryIDPrefix = "sum_"
	FileIDPrefix    = "file_"
)

// ContextEntry is a single entry in the LCM context (either a message or summary).
type ContextEntry struct {
	Position   int64
	ItemType   string // "message" or "summary"
	MessageID  string // set when ItemType == "message"
	SummaryID  string // set when ItemType == "summary"
	TokenCount int64
	// Fields populated for summaries.
	SummaryContent string
	SummaryKind    string
	ParentIDs      []string // only for condensed summaries
	BlockID        string   // for reversible summaries with block IDs
}

// CompactionResult is returned by compaction operations.
type CompactionResult struct {
	Rounds      int
	ActionTaken bool
	TokenCount  int64
}

// CompactionEventType categorizes a compaction event.
type CompactionEventType string

const (
	CompactionStarted   CompactionEventType = "started"
	CompactionCompleted CompactionEventType = "completed"
	CompactionFailed    CompactionEventType = "failed"
)

// CompactionEvent is published on the event bus when compaction state changes.
type CompactionEvent struct {
	Type      CompactionEventType
	SessionID string
	Blocking  bool   // true if this is a hard-limit blocking compaction
	Rounds    int    // for completed events
	Success   bool   // for completed events
	Error     string // for failed events
}

// ThresholdCheck is the result of checking if a session is over the threshold.
type ThresholdCheck struct {
	CurrentTokens int64
	SoftLimit     int64
	HardLimit     int64
	OverSoft      bool
	OverHard      bool
}

// SummaryInput is the input to the summarization algorithm.
type SummaryInput struct {
	SessionID   string
	Messages    []MessageForSummary
	SummaryText string // populated for fallback cases
}

// MessageForSummary is a simplified message for summarization.
type MessageForSummary struct {
	ID        string
	SessionID string
	Seq       int64
	Role      string
	Content   string // extracted text content
}

// SummarySearchResult is a lightweight result from FTS search.
type SummarySearchResult struct {
	SummaryID string
	Kind      string
}

// LargeFileSearchResult is a ranked result from large file FTS search.
type LargeFileSearchResult struct {
	FileID  string
	Path    string
	Rank    float64
	Snippet string
}

// RankedSearchResult is a result from ranked FTS5 search with bm25() scoring
// and snippet highlighting.
type RankedSearchResult struct {
	RowID     int64
	SummaryID string
	Rank      float64
	Snippet   string
}

// TimeQueryMessage is a message returned by time-range queries.
type TimeQueryMessage struct {
	ID        string
	SessionID string
	Seq       int64
	Role      string
	Content   string
	CreatedAt int64
}

// Budget holds the computed token budget for a session.
type Budget struct {
	SoftThreshold int64
	HardLimit     int64
	ContextWindow int64
}

// ActiveContext holds the active context overview for a session.
type ActiveContext struct {
	SessionID   string
	Entries     []ContextEntry
	TotalTokens int64
	EntryCount  int
}

// ContextFilter filters active context results.
type ContextFilter struct {
	Type      *string // filter by item_type ("message" or "summary")
	MinTokens *int    // minimum token_count
	MaxTokens *int    // maximum token_count
}

// LineageDirection specifies which direction to traverse the summary DAG.
type LineageDirection int

const (
	// LineageAncestors walks parents upward from the target summary.
	LineageAncestors LineageDirection = iota
	// LineageDescendants walks children downward from the target summary.
	LineageDescendants
	// LineageBoth combines both ancestor and descendant traversal.
	LineageBoth
)

// LineageNode represents a single node in a lineage traversal result.
type LineageNode struct {
	SummaryID  string
	ParentID   string // empty for the root node of the traversal
	Depth      int
	TokenCount int64
	Kind       string
}
