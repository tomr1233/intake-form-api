package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/tomr1233/intake-form-api/internal/models"
	"google.golang.org/api/option"
)

const systemInstruction = "You are an expert sales analyst. Be direct, critical, and strategic. Do not fluff the response."

const promptIntro = `You are a world-class business consultant and sales strategist. Analyze this intake form and provide strategic insights.

Note: Only the information provided below was submitted. Analyze based on the available data. If critical information is missing, note that in your analysis.
`

const promptOutro = `
---

Analyze this prospect and provide a JSON response with:
1. executiveSummary: 2-3 sentence summary of the prospect
2. clientPsychology: Analysis of their mindset and what drives them
3. operationalGapAnalysis: What gaps exist in their current operations
4. redFlags: Array of disqualifying risks or concerns (strings)
5. greenFlags: Array of positive indicators (strings)
6. strategicQuestions: Array of 3-5 questions to ask on the sales call (strings)
7. closingStrategy: How to approach closing this prospect
8. estimatedFitScore: Number from 0-100 indicating fit quality`

// GeminiClient wraps the Gemini AI client.
type GeminiClient struct {
	client *genai.Client
	model  *genai.GenerativeModel
}

// NewGeminiClient creates a new Gemini AI client.
func NewGeminiClient(ctx context.Context, apiKey string) (*GeminiClient, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("creating genai client: %w", err)
	}

	model := client.GenerativeModel("gemini-2.5-flash")
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(systemInstruction),
		},
	}
	model.SetTemperature(0.3)
	model.ResponseMIMEType = "application/json"

	// Set response schema for structured output
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"executiveSummary": {
				Type:        genai.TypeString,
				Description: "A 2-3 sentence executive summary of the prospect",
			},
			"clientPsychology": {
				Type:        genai.TypeString,
				Description: "Analysis of the client's mindset and motivations",
			},
			"operationalGapAnalysis": {
				Type:        genai.TypeString,
				Description: "Analysis of gaps in their current operations",
			},
			"redFlags": {
				Type:        genai.TypeArray,
				Description: "Array of disqualifying risks or concerns",
				Items:       &genai.Schema{Type: genai.TypeString},
			},
			"greenFlags": {
				Type:        genai.TypeArray,
				Description: "Array of positive indicators",
				Items:       &genai.Schema{Type: genai.TypeString},
			},
			"strategicQuestions": {
				Type:        genai.TypeArray,
				Description: "3-5 strategic questions to ask on the sales call",
				Items:       &genai.Schema{Type: genai.TypeString},
			},
			"closingStrategy": {
				Type:        genai.TypeString,
				Description: "Recommended approach for closing this prospect",
			},
			"estimatedFitScore": {
				Type:        genai.TypeInteger,
				Description: "Fit score from 0-100",
			},
		},
		Required: []string{
			"executiveSummary",
			"clientPsychology",
			"operationalGapAnalysis",
			"redFlags",
			"greenFlags",
			"strategicQuestions",
			"closingStrategy",
			"estimatedFitScore",
		},
	}

	return &GeminiClient{client: client, model: model}, nil
}

// Close closes the Gemini client.
func (g *GeminiClient) Close() error {
	return g.client.Close()
}

// Analyze sends the submission data to Gemini and returns the analysis result.
func (g *GeminiClient) Analyze(ctx context.Context, s *models.Submission) (*models.AnalysisResult, error) {
	prompt := g.buildPrompt(s)

	resp, err := g.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("generating content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from Gemini")
	}

	// Extract the text response
	textPart, ok := resp.Candidates[0].Content.Parts[0].(genai.Text)
	if !ok {
		return nil, fmt.Errorf("unexpected response type from Gemini")
	}

	// Parse JSON response
	var result struct {
		ExecutiveSummary       string   `json:"executiveSummary"`
		ClientPsychology       string   `json:"clientPsychology"`
		OperationalGapAnalysis string   `json:"operationalGapAnalysis"`
		RedFlags               []string `json:"redFlags"`
		GreenFlags             []string `json:"greenFlags"`
		StrategicQuestions     []string `json:"strategicQuestions"`
		ClosingStrategy        string   `json:"closingStrategy"`
		EstimatedFitScore      int      `json:"estimatedFitScore"`
	}

	if err := json.Unmarshal([]byte(textPart), &result); err != nil {
		return nil, fmt.Errorf("parsing Gemini response: %w", err)
	}

	// Store raw response for debugging
	var rawResponse map[string]interface{}
	_ = json.Unmarshal([]byte(textPart), &rawResponse)

	return &models.AnalysisResult{
		SubmissionID:           s.ID,
		ExecutiveSummary:       result.ExecutiveSummary,
		ClientPsychology:       result.ClientPsychology,
		OperationalGapAnalysis: result.OperationalGapAnalysis,
		RedFlags:               result.RedFlags,
		GreenFlags:             result.GreenFlags,
		StrategicQuestions:     result.StrategicQuestions,
		ClosingStrategy:        result.ClosingStrategy,
		EstimatedFitScore:      result.EstimatedFitScore,
		RawResponse:            rawResponse,
	}, nil
}

func (g *GeminiClient) buildPrompt(s *models.Submission) string {
	var b strings.Builder
	b.WriteString(promptIntro)

	// Prospect Information
	var contact []string
	if s.FirstName != "" || s.LastName != "" {
		contact = append(contact, fmt.Sprintf("**Contact:** %s %s", s.FirstName, s.LastName))
	}
	if s.Email != "" {
		contact = append(contact, fmt.Sprintf("**Email:** %s", s.Email))
	}
	if s.CompanyName != "" {
		contact = append(contact, fmt.Sprintf("**Company:** %s", s.CompanyName))
	}
	if s.Website != "" {
		contact = append(contact, fmt.Sprintf("**Website:** %s", s.Website))
	}
	if len(contact) > 0 {
		b.WriteString("\n## Prospect Information\n")
		b.WriteString(strings.Join(contact, "\n"))
		b.WriteString("\n")
	}

	// Why They're Booking
	if s.ReasonForBooking != "" {
		b.WriteString("\n## Why They're Booking\n")
		b.WriteString(s.ReasonForBooking)
		b.WriteString("\n")
	}

	// How They Found Us
	if s.HowDidYouHear != "" {
		b.WriteString("\n## How They Found Us\n")
		b.WriteString(s.HowDidYouHear)
		b.WriteString("\n")
	}

	// Financials
	var financials []string
	if s.CurrentRevenue != "" {
		financials = append(financials, fmt.Sprintf("- Current Revenue: %s", s.CurrentRevenue))
	}
	if s.TeamSize != "" {
		financials = append(financials, fmt.Sprintf("- Team Size: %s", s.TeamSize))
	}
	if s.AverageDealSize != "" {
		financials = append(financials, fmt.Sprintf("- Average Deal Size: %s", s.AverageDealSize))
	}
	if s.MarketingBudget != "" {
		financials = append(financials, fmt.Sprintf("- Marketing Budget: %s", s.MarketingBudget))
	}
	if len(financials) > 0 {
		b.WriteString("\n## Financials\n")
		b.WriteString(strings.Join(financials, "\n"))
		b.WriteString("\n")
	}

	// Their Business
	var business []string
	if s.PrimaryService != "" {
		business = append(business, fmt.Sprintf("- Primary Service: %s", s.PrimaryService))
	}
	if s.IsDecisionMaker != "" {
		business = append(business, fmt.Sprintf("- Decision Maker: %s", s.IsDecisionMaker))
	}
	if s.AcquisitionSource != "" {
		business = append(business, fmt.Sprintf("- Acquisition Source: %s", s.AcquisitionSource))
	}
	if s.SalesProcess != "" {
		business = append(business, fmt.Sprintf("- Sales Process: %s", s.SalesProcess))
	}
	if s.FulfillmentWorkflow != "" {
		business = append(business, fmt.Sprintf("- Fulfillment Workflow: %s", s.FulfillmentWorkflow))
	}
	if s.CurrentTechStack != "" {
		business = append(business, fmt.Sprintf("- Current Tech Stack: %s", s.CurrentTechStack))
	}
	if len(business) > 0 {
		b.WriteString("\n## Their Business\n")
		b.WriteString(strings.Join(business, "\n"))
		b.WriteString("\n")
	}

	// Current Challenges
	if s.BiggestBottleneck != "" {
		b.WriteString("\n## Current Challenges\n")
		b.WriteString(s.BiggestBottleneck)
		b.WriteString("\n")
	}

	// Previous Agency Experience
	if s.PreviousAgencyExperience != "" {
		b.WriteString("\n## Previous Agency Experience\n")
		b.WriteString(s.PreviousAgencyExperience)
		b.WriteString("\n")
	}

	// Future Goals
	var goals []string
	if s.DesiredOutcome != "" {
		goals = append(goals, fmt.Sprintf("- Desired Outcome: %s", s.DesiredOutcome))
	}
	if s.DesiredSpeed != "" {
		goals = append(goals, fmt.Sprintf("- Desired Speed: %s", s.DesiredSpeed))
	}
	if s.ReadyToScale != "" {
		goals = append(goals, fmt.Sprintf("- Ready to Scale: %s", s.ReadyToScale))
	}
	if len(goals) > 0 {
		b.WriteString("\n## Future Goals\n")
		b.WriteString(strings.Join(goals, "\n"))
		b.WriteString("\n")
	}

	b.WriteString(promptOutro)
	return b.String()
}
