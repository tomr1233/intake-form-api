package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tomr1233/intake-form-api/internal/database"
	"github.com/tomr1233/intake-form-api/internal/models"
)

type AnalysisRepository interface {
	Create(ctx context.Context, a *models.AnalysisResult) error
	GetBySubmissionID(ctx context.Context, submissionID uuid.UUID) (*models.AnalysisResult, error)
}

type analysisRepo struct {
	db *database.DB
}

func NewAnalysisRepository(db *database.DB) AnalysisRepository {
	return &analysisRepo{db: db}
}

func (r *analysisRepo) Create(ctx context.Context, a *models.AnalysisResult) error {
	// Marshal arrays to JSON
	redFlagsJSON, err := json.Marshal(a.RedFlags)
	if err != nil {
		return fmt.Errorf("marshaling red_flags: %w", err)
	}

	greenFlagsJSON, err := json.Marshal(a.GreenFlags)
	if err != nil {
		return fmt.Errorf("marshaling green_flags: %w", err)
	}

	strategicQuestionsJSON, err := json.Marshal(a.StrategicQuestions)
	if err != nil {
		return fmt.Errorf("marshaling strategic_questions: %w", err)
	}

	var rawResponseJSON []byte
	if a.RawResponse != nil {
		rawResponseJSON, err = json.Marshal(a.RawResponse)
		if err != nil {
			return fmt.Errorf("marshaling raw_response: %w", err)
		}
	}

	query := `
		INSERT INTO analysis_results (
			id, submission_id,
			executive_summary, client_psychology, operational_gap_analysis,
			red_flags, green_flags, strategic_questions,
			closing_strategy, estimated_fit_score,
			raw_response, error_message,
			created_at
		) VALUES (
			$1, $2,
			$3, $4, $5,
			$6, $7, $8,
			$9, $10,
			$11, $12,
			$13
		)`

	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}

	var fitScore *int
	if a.EstimatedFitScore > 0 {
		fitScore = &a.EstimatedFitScore
	}

	var errorMsg *string
	if a.ErrorMessage != "" {
		errorMsg = &a.ErrorMessage
	}

	_, err = r.db.Pool.Exec(ctx, query,
		a.ID, a.SubmissionID,
		a.ExecutiveSummary, a.ClientPsychology, a.OperationalGapAnalysis,
		redFlagsJSON, greenFlagsJSON, strategicQuestionsJSON,
		a.ClosingStrategy, fitScore,
		rawResponseJSON, errorMsg,
		a.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting analysis result: %w", err)
	}

	return nil
}

func (r *analysisRepo) GetBySubmissionID(ctx context.Context, submissionID uuid.UUID) (*models.AnalysisResult, error) {
	query := `
		SELECT
			id, submission_id,
			executive_summary, client_psychology, operational_gap_analysis,
			red_flags, green_flags, strategic_questions,
			closing_strategy, estimated_fit_score,
			raw_response, error_message,
			created_at
		FROM analysis_results
		WHERE submission_id = $1`

	var a models.AnalysisResult
	var redFlagsJSON, greenFlagsJSON, strategicQuestionsJSON, rawResponseJSON []byte
	var executiveSummary, clientPsychology, operationalGapAnalysis, closingStrategy *string
	var fitScore *int
	var errorMessage *string

	err := r.db.Pool.QueryRow(ctx, query, submissionID).Scan(
		&a.ID, &a.SubmissionID,
		&executiveSummary, &clientPsychology, &operationalGapAnalysis,
		&redFlagsJSON, &greenFlagsJSON, &strategicQuestionsJSON,
		&closingStrategy, &fitScore,
		&rawResponseJSON, &errorMessage,
		&a.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying analysis result: %w", err)
	}

	// Handle nullable fields
	a.ExecutiveSummary = derefString(executiveSummary)
	a.ClientPsychology = derefString(clientPsychology)
	a.OperationalGapAnalysis = derefString(operationalGapAnalysis)
	a.ClosingStrategy = derefString(closingStrategy)
	if fitScore != nil {
		a.EstimatedFitScore = *fitScore
	}
	if errorMessage != nil {
		a.ErrorMessage = *errorMessage
	}

	// Unmarshal JSON arrays
	if len(redFlagsJSON) > 0 {
		if err := json.Unmarshal(redFlagsJSON, &a.RedFlags); err != nil {
			return nil, fmt.Errorf("unmarshaling red_flags: %w", err)
		}
	}
	if len(greenFlagsJSON) > 0 {
		if err := json.Unmarshal(greenFlagsJSON, &a.GreenFlags); err != nil {
			return nil, fmt.Errorf("unmarshaling green_flags: %w", err)
		}
	}
	if len(strategicQuestionsJSON) > 0 {
		if err := json.Unmarshal(strategicQuestionsJSON, &a.StrategicQuestions); err != nil {
			return nil, fmt.Errorf("unmarshaling strategic_questions: %w", err)
		}
	}
	if len(rawResponseJSON) > 0 {
		if err := json.Unmarshal(rawResponseJSON, &a.RawResponse); err != nil {
			return nil, fmt.Errorf("unmarshaling raw_response: %w", err)
		}
	}

	return &a, nil
}
