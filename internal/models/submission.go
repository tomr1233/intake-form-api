package models

import (
	"time"

	"github.com/google/uuid"
)

type SubmissionStatus string

const (
	StatusPending    SubmissionStatus = "pending"
	StatusProcessing SubmissionStatus = "processing"
	StatusCompleted  SubmissionStatus = "completed"
	StatusFailed     SubmissionStatus = "failed"
)

// Submission represents an intake form submission.
type Submission struct {
	ID         uuid.UUID        `json:"id"`
	AdminToken string           `json:"-"` // Never expose in JSON
	Status     SubmissionStatus `json:"status"`

	// Contact info
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Email       string `json:"email"`
	Website     string `json:"website"`
	CompanyName string `json:"companyName"`

	// Context
	ReasonForBooking string `json:"reasonForBooking"`
	HowDidYouHear    string `json:"howDidYouHear"`

	// Current reality
	CurrentRevenue           string `json:"currentRevenue"`
	TeamSize                 string `json:"teamSize"`
	PrimaryService           string `json:"primaryService"`
	AverageDealSize          string `json:"averageDealSize"`
	MarketingBudget          string `json:"marketingBudget"`
	BiggestBottleneck        string `json:"biggestBottleneck"`
	IsDecisionMaker          string `json:"isDecisionMaker"`
	PreviousAgencyExperience string `json:"previousAgencyExperience"`

	// Process fields
	AcquisitionSource   string `json:"acquisitionSource"`
	SalesProcess        string `json:"salesProcess"`
	FulfillmentWorkflow string `json:"fulfillmentWorkflow"`
	CurrentTechStack    string `json:"currentTechStack"`

	// Dream future
	DesiredOutcome string `json:"desiredOutcome"`
	DesiredSpeed   string `json:"desiredSpeed"`
	ReadyToScale   string `json:"readyToScale"`

	// Timestamps
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CreateSubmissionRequest represents the JSON request body for creating a submission.
type CreateSubmissionRequest struct {
	FirstName                string `json:"firstName" binding:"max=100"`
	LastName                 string `json:"lastName" binding:"max=100"`
	Email                    string `json:"email" binding:"omitempty,email,max=255"`
	Website                  string `json:"website" binding:"max=255"`
	CompanyName              string `json:"companyName" binding:"max=255"`
	ReasonForBooking         string `json:"reasonForBooking"`
	HowDidYouHear            string `json:"howDidYouHear" binding:"max=100"`
	CurrentRevenue           string `json:"currentRevenue" binding:"max=50"`
	TeamSize                 string `json:"teamSize" binding:"max=50"`
	PrimaryService           string `json:"primaryService"`
	AverageDealSize          string `json:"averageDealSize" binding:"max=50"`
	MarketingBudget          string `json:"marketingBudget" binding:"max=50"`
	BiggestBottleneck        string `json:"biggestBottleneck"`
	IsDecisionMaker          string `json:"isDecisionMaker" binding:"max=20"`
	PreviousAgencyExperience string `json:"previousAgencyExperience"`
	AcquisitionSource        string `json:"acquisitionSource"`
	SalesProcess             string `json:"salesProcess"`
	FulfillmentWorkflow      string `json:"fulfillmentWorkflow"`
	CurrentTechStack         string `json:"currentTechStack"`
	DesiredOutcome           string `json:"desiredOutcome"`
	DesiredSpeed             string `json:"desiredSpeed" binding:"max=50"`
	ReadyToScale             string `json:"readyToScale" binding:"max=20"`
}

// ToSubmission converts a CreateSubmissionRequest to a Submission model.
func (r *CreateSubmissionRequest) ToSubmission(id uuid.UUID, adminToken string) *Submission {
	now := time.Now()
	return &Submission{
		ID:                       id,
		AdminToken:               adminToken,
		Status:                   StatusPending,
		FirstName:                r.FirstName,
		LastName:                 r.LastName,
		Email:                    r.Email,
		Website:                  r.Website,
		CompanyName:              r.CompanyName,
		ReasonForBooking:         r.ReasonForBooking,
		HowDidYouHear:            r.HowDidYouHear,
		CurrentRevenue:           r.CurrentRevenue,
		TeamSize:                 r.TeamSize,
		PrimaryService:           r.PrimaryService,
		AverageDealSize:          r.AverageDealSize,
		MarketingBudget:          r.MarketingBudget,
		BiggestBottleneck:        r.BiggestBottleneck,
		IsDecisionMaker:          r.IsDecisionMaker,
		PreviousAgencyExperience: r.PreviousAgencyExperience,
		AcquisitionSource:        r.AcquisitionSource,
		SalesProcess:             r.SalesProcess,
		FulfillmentWorkflow:      r.FulfillmentWorkflow,
		CurrentTechStack:         r.CurrentTechStack,
		DesiredOutcome:           r.DesiredOutcome,
		DesiredSpeed:             r.DesiredSpeed,
		ReadyToScale:             r.ReadyToScale,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
}
