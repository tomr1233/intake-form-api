package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"github.com/tomr1233/intake-form-api/internal/models"
	"google.golang.org/api/option"
)

const systemInstruction = "You are an expert sales analyst. Be direct, critical, and strategic. Do not fluff the response."

const promptTemplate = `You are a world-class business consultant and sales strategist. Analyze this intake form and provide strategic insights.

## Prospect Information

**Contact:** %s %s (%s)
**Company:** %s
**Website:** %s

## Why They're Booking
%s

## How They Found Us
%s

## Financials
- Current Revenue: %s
- Team Size: %s
- Average Deal Size: %s
- Marketing Budget: %s

## Their Business
- Primary Service: %s
- Decision Maker: %s

## Current Challenges
%s

## Previous Agency Experience
%s

## Future Goals
- Desired Outcome: %s
- Desired Speed: %s
- Ready to Scale: %s

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
	return fmt.Sprintf(promptTemplate,
		s.FirstName, s.LastName, s.Email,
		s.CompanyName,
		s.Website,
		s.ReasonForBooking,
		s.HowDidYouHear,
		s.CurrentRevenue,
		s.TeamSize,
		s.AverageDealSize,
		s.MarketingBudget,
		s.PrimaryService,
		s.IsDecisionMaker,
		s.BiggestBottleneck,
		s.PreviousAgencyExperience,
		s.DesiredOutcome,
		s.DesiredSpeed,
		s.ReadyToScale,
	)
}
