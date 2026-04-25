package modality

import "context"

const (
	AudioNoteStatusQueued  = "queued"
	AudioNoteStatusRunning = "running"
	AudioNoteStatusSuccess = "success"
	AudioNoteStatusFailed  = "failed"
)

type AudioNotesAdapter interface {
	SubmitNotes(ctx context.Context, req *AudioNoteRequest) (*AudioNoteJob, error)
	GetAudioNote(ctx context.Context, req *AudioNoteStatusRequest) (*AudioNoteJob, error)
}

type AudioNoteRequest struct {
	Model              string          `json:"model"`
	Routing            *RoutingOptions `json:"routing,omitempty"`
	SourceURL          string          `json:"source_url"`
	FileType           string          `json:"file_type,omitempty"`
	Language           string          `json:"language,omitempty"`
	IncludeSummary     bool            `json:"include_summary,omitempty"`
	IncludeChapters    bool            `json:"include_chapters,omitempty"`
	IncludeActionItems bool            `json:"include_action_items,omitempty"`
	IncludeQAPairs     bool            `json:"include_qa_pairs,omitempty"`
	TargetLanguage     string          `json:"target_language,omitempty"`
}

type AudioNoteStatusRequest struct {
	Model  string `json:"model"`
	TaskID string `json:"task_id"`
}

type AudioNoteJob struct {
	ID     string           `json:"id"`
	Object string           `json:"object"`
	Model  string           `json:"model"`
	Status string           `json:"status"`
	Result *AudioNoteResult `json:"result,omitempty"`
	Error  *AudioError      `json:"error,omitempty"`
}

type AudioNoteResult struct {
	Transcript  string                `json:"transcript,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Chapters    []AudioNoteChapter    `json:"chapters,omitempty"`
	ActionItems []AudioNoteActionItem `json:"action_items,omitempty"`
	QAPairs     []AudioNoteQAPair     `json:"qa_pairs,omitempty"`
	Translation string                `json:"translation,omitempty"`
	Metadata    map[string]any        `json:"metadata,omitempty"`
}

type AudioNoteChapter struct {
	Title string  `json:"title,omitempty"`
	Start float64 `json:"start,omitempty"`
	End   float64 `json:"end,omitempty"`
	Text  string  `json:"text,omitempty"`
}

type AudioNoteActionItem struct {
	Content   string   `json:"content,omitempty"`
	Executor  []string `json:"executor,omitempty"`
	Due       []string `json:"due,omitempty"`
	StartTime float64  `json:"start_time,omitempty"`
}

type AudioNoteQAPair struct {
	Question string `json:"question,omitempty"`
	Answer   string `json:"answer,omitempty"`
}
