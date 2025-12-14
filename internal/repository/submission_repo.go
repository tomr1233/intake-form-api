package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tomr1233/intake-form-api/internal/database"
	"github.com/tomr1233/intake-form-api/internal/models"
)

var ErrNotFound = errors.New("resource not found")

type SubmissionRepository interface {
	Create(ctx context.Context, s *models.Submission) error
	GetByAdminToken(ctx context.Context, token string) (*models.Submission, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.SubmissionStatus) error
}

type submissionRepo struct {
	db *database.DB
}

func NewSubmissionRepository(db *database.DB) SubmissionRepository {
	return &submissionRepo{db: db}
}

func (r *submissionRepo) Create(ctx context.Context, s *models.Submission) error {
	query := `
		INSERT INTO submissions (
			id, admin_token, status,
			first_name, last_name, email, website, company_name,
			reason_for_booking, how_did_you_hear,
			current_revenue, team_size, primary_service, average_deal_size,
			marketing_budget, biggest_bottleneck, is_decision_maker, previous_agency_experience,
			acquisition_source, sales_process, fulfillment_workflow, current_tech_stack,
			desired_outcome, desired_speed, ready_to_scale,
			created_at, updated_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6, $7, $8,
			$9, $10,
			$11, $12, $13, $14,
			$15, $16, $17, $18,
			$19, $20, $21, $22,
			$23, $24, $25,
			$26, $27
		)`

	_, err := r.db.Pool.Exec(ctx, query,
		s.ID, s.AdminToken, s.Status,
		s.FirstName, s.LastName, s.Email, s.Website, s.CompanyName,
		s.ReasonForBooking, s.HowDidYouHear,
		s.CurrentRevenue, s.TeamSize, s.PrimaryService, s.AverageDealSize,
		s.MarketingBudget, s.BiggestBottleneck, s.IsDecisionMaker, s.PreviousAgencyExperience,
		s.AcquisitionSource, s.SalesProcess, s.FulfillmentWorkflow, s.CurrentTechStack,
		s.DesiredOutcome, s.DesiredSpeed, s.ReadyToScale,
		s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting submission: %w", err)
	}

	return nil
}

func (r *submissionRepo) GetByAdminToken(ctx context.Context, token string) (*models.Submission, error) {
	query := `
		SELECT
			id, admin_token, status,
			first_name, last_name, email, website, company_name,
			reason_for_booking, how_did_you_hear,
			current_revenue, team_size, primary_service, average_deal_size,
			marketing_budget, biggest_bottleneck, is_decision_maker, previous_agency_experience,
			acquisition_source, sales_process, fulfillment_workflow, current_tech_stack,
			desired_outcome, desired_speed, ready_to_scale,
			created_at, updated_at
		FROM submissions
		WHERE admin_token = $1`

	var s models.Submission
	var lastName, website, companyName, reasonForBooking, howDidYouHear *string
	var currentRevenue, teamSize, primaryService, averageDealSize *string
	var marketingBudget, biggestBottleneck, isDecisionMaker, previousAgencyExperience *string
	var acquisitionSource, salesProcess, fulfillmentWorkflow, currentTechStack *string
	var desiredOutcome, desiredSpeed, readyToScale *string

	err := r.db.Pool.QueryRow(ctx, query, token).Scan(
		&s.ID, &s.AdminToken, &s.Status,
		&s.FirstName, &lastName, &s.Email, &website, &companyName,
		&reasonForBooking, &howDidYouHear,
		&currentRevenue, &teamSize, &primaryService, &averageDealSize,
		&marketingBudget, &biggestBottleneck, &isDecisionMaker, &previousAgencyExperience,
		&acquisitionSource, &salesProcess, &fulfillmentWorkflow, &currentTechStack,
		&desiredOutcome, &desiredSpeed, &readyToScale,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying submission: %w", err)
	}

	// Handle nullable fields
	s.LastName = derefString(lastName)
	s.Website = derefString(website)
	s.CompanyName = derefString(companyName)
	s.ReasonForBooking = derefString(reasonForBooking)
	s.HowDidYouHear = derefString(howDidYouHear)
	s.CurrentRevenue = derefString(currentRevenue)
	s.TeamSize = derefString(teamSize)
	s.PrimaryService = derefString(primaryService)
	s.AverageDealSize = derefString(averageDealSize)
	s.MarketingBudget = derefString(marketingBudget)
	s.BiggestBottleneck = derefString(biggestBottleneck)
	s.IsDecisionMaker = derefString(isDecisionMaker)
	s.PreviousAgencyExperience = derefString(previousAgencyExperience)
	s.AcquisitionSource = derefString(acquisitionSource)
	s.SalesProcess = derefString(salesProcess)
	s.FulfillmentWorkflow = derefString(fulfillmentWorkflow)
	s.CurrentTechStack = derefString(currentTechStack)
	s.DesiredOutcome = derefString(desiredOutcome)
	s.DesiredSpeed = derefString(desiredSpeed)
	s.ReadyToScale = derefString(readyToScale)

	return &s, nil
}

func (r *submissionRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status models.SubmissionStatus) error {
	query := `UPDATE submissions SET status = $1, updated_at = $2 WHERE id = $3`

	result, err := r.db.Pool.Exec(ctx, query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("updating submission status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
