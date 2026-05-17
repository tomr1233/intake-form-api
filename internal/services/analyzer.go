package services

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/tomr1233/intake-form-api/internal/models"
	"github.com/tomr1233/intake-form-api/internal/observability"
	"github.com/tomr1233/intake-form-api/internal/repository"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Analyzer handles async analysis of submissions.
type Analyzer struct {
	gemini      *GeminiClient
	submissions repository.SubmissionRepository
	analysis    repository.AnalysisRepository
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(
	gemini *GeminiClient,
	submissions repository.SubmissionRepository,
	analysis repository.AnalysisRepository,
) *Analyzer {
	return &Analyzer{
		gemini:      gemini,
		submissions: submissions,
		analysis:    analysis,
	}
}

// AnalyzeAsync triggers async analysis of a submission.
// This spawns a goroutine that:
// 1. Updates status to "processing"
// 2. Calls Gemini API
// 3. Stores the result
// 4. Updates status to "completed" or "failed"
func (a *Analyzer) AnalyzeAsync(submission *models.Submission) {
	go func() {
		// The HTTP request span has already ended by the time this goroutine
		// runs, so start a fresh root trace for the async analysis. The
		// submission ID is exposed as a Langfuse session ID so all attempts
		// for a submission group together.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		ctx, span := observability.Tracer().Start(ctx, "analyze-submission")
		defer span.End()
		span.SetAttributes(
			attribute.String(observability.AttrSessionID, submission.ID.String()),
			attribute.String("submission.id", submission.ID.String()),
			attribute.String("submission.company", submission.CompanyName),
		)
		// Trace-level Input panel in Langfuse is populated from the root
		// span's observation.input, so serialize the submission here.
		if submissionJSON, err := json.Marshal(submission); err == nil {
			observability.SetObservationInput(span, string(submissionJSON))
		}

		log.Printf("Starting analysis for submission %s", submission.ID)

		// Update status to processing
		if err := a.submissions.UpdateStatus(ctx, submission.ID, models.StatusProcessing); err != nil {
			log.Printf("ERROR: failed to update status to processing for %s: %v", submission.ID, err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "update status: processing")
			return
		}

		// Call Gemini API
		result, err := a.gemini.Analyze(ctx, submission)
		if err != nil {
			log.Printf("ERROR: Gemini analysis failed for %s: %v", submission.ID, err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "gemini analyze")

			// Save error and mark as failed
			errorResult := &models.AnalysisResult{
				SubmissionID: submission.ID,
				ErrorMessage: err.Error(),
			}
			if createErr := a.analysis.Create(ctx, errorResult); createErr != nil {
				log.Printf("ERROR: failed to save error result for %s: %v", submission.ID, createErr)
			}

			if updateErr := a.submissions.UpdateStatus(ctx, submission.ID, models.StatusFailed); updateErr != nil {
				log.Printf("ERROR: failed to update status to failed for %s: %v", submission.ID, updateErr)
			}
			return
		}

		// Save result
		if err := a.analysis.Create(ctx, result); err != nil {
			log.Printf("ERROR: failed to save analysis result for %s: %v", submission.ID, err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "save analysis result")
			a.submissions.UpdateStatus(ctx, submission.ID, models.StatusFailed)
			return
		}

		// Mark as completed
		if err := a.submissions.UpdateStatus(ctx, submission.ID, models.StatusCompleted); err != nil {
			log.Printf("ERROR: failed to update status to completed for %s: %v", submission.ID, err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "update status: completed")
			return
		}

		span.SetAttributes(attribute.Int("analysis.fit_score", result.EstimatedFitScore))
		observability.SetObservationOutput(span, result.ExecutiveSummary)

		log.Printf("Analysis completed for submission %s (fit score: %d)", submission.ID, result.EstimatedFitScore)
	}()
}
