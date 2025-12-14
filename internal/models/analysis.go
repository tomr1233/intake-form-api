package models

import (
	"time"

	"github.com/google/uuid"
)

// AnalysisResult represents the AI-generated analysis of a submission.
type AnalysisResult struct {
	ID           uuid.UUID `json:"id"`
	SubmissionID uuid.UUID `json:"submissionId"`

	ExecutiveSummary       string   `json:"executiveSummary"`
	ClientPsychology       string   `json:"clientPsychology"`
	OperationalGapAnalysis string   `json:"operationalGapAnalysis"`
	RedFlags               []string `json:"redFlags"`
	GreenFlags             []string `json:"greenFlags"`
	StrategicQuestions     []string `json:"strategicQuestions"`
	ClosingStrategy        string   `json:"closingStrategy"`
	EstimatedFitScore      int      `json:"estimatedFitScore"`

	RawResponse  map[string]interface{} `json:"-"` // Raw Gemini response for debugging
	ErrorMessage string                 `json:"-"` // Error message if analysis failed

	CreatedAt time.Time `json:"createdAt"`
}

// AdminResponse is the response format for the admin endpoint.
type AdminResponse struct {
	Submission *Submission      `json:"submission"`
	Analysis   *AnalysisResult  `json:"analysis"`
	Status     SubmissionStatus `json:"status"`
	CreatedAt  time.Time        `json:"createdAt"`
}

// AdminStatusResponse is the lightweight status polling response.
type AdminStatusResponse struct {
	Status            SubmissionStatus `json:"status"`
	EstimatedFitScore *int             `json:"estimatedFitScore,omitempty"`
}
