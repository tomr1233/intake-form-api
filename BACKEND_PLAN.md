# Go Backend Implementation Plan

## Overview

Add a Go backend to the Clarity intake form wizard that:
1. Accepts form submissions instantly and stores them in PostgreSQL
2. Processes Gemini AI analysis asynchronously (simple goroutine)
3. Shows users a thank you page immediately after submission
4. Generates unique admin links to view AI-generated results

**Architecture Decisions:**
- **Async processing**: Simple goroutines (jobs lost on restart, acceptable for this use case)
- **Admin UI**: Same React app with `/admin/:token` route, reuse AnalysisDashboard
- **Deployment**: Separate - frontend on Vercel/Netlify, Go backend on separate host

---

## Current Frontend State

The frontend is a React + TypeScript + Vite app that:
- Collects 21 form fields across 4 wizard steps (Basics, Current Reality, Dream Future)
- Calls Gemini 2.5 Flash directly from the browser for AI analysis
- Displays results in an AnalysisDashboard component

**Key Types (from `types.ts`):**

```typescript
// IntakeFormData - 21 fields
interface IntakeFormData {
  // Contact info
  firstName: string;
  lastName: string;
  email: string;
  website: string;
  companyName: string;

  // Context
  reasonForBooking: string;
  howDidYouHear: string;

  // Current reality
  currentRevenue: string;
  teamSize: string;
  primaryService: string;
  averageDealSize: string;
  marketingBudget: string;
  biggestBottleneck: string;
  isDecisionMaker: string;
  previousAgencyExperience: string;

  // Process fields
  acquisitionSource: string;
  salesProcess: string;
  fulfillmentWorkflow: string;
  currentTechStack: string;

  // Dream future
  desiredOutcome: string;
  desiredSpeed: string;
  readyToScale: string;
}

// AnalysisResult - 8 fields from Gemini AI
interface AnalysisResult {
  executiveSummary: string;
  clientPsychology: string;
  operationalGapAnalysis: string;
  redFlags: string[];
  greenFlags: string[];
  strategicQuestions: string[];
  closingStrategy: string;
  estimatedFitScore: number; // 0-100
}
```

---

## Architecture

```
┌─────────────┐     POST /api/submissions      ┌──────────────┐
│   React     │ ──────────────────────────────▶│   Go API     │
│   Frontend  │◀────────── { id, adminToken }──│   Server     │
└─────────────┘                                └──────┬───────┘
       │                                              │
       │ Redirect to /thank-you                       │ Async goroutine
       ▼                                              ▼
┌─────────────┐                                ┌──────────────┐
│  Thank You  │                                │   Gemini     │
│    Page     │                                │   AI Call    │
└─────────────┘                                └──────┬───────┘
                                                      │
       ┌──────────────────────────────────────────────┘
       │ Store results
       ▼
┌──────────────┐     GET /api/admin/{token}    ┌──────────────┐
│  PostgreSQL  │◀─────────────────────────────▶│ Admin View   │
│   Database   │                                │   (Results)  │
└──────────────┘                                └──────────────┘
```

---

## Database Schema

```sql
-- Submissions table
CREATE TABLE submissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_token VARCHAR(64) UNIQUE NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, processing, completed, failed

    -- Contact info
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100),
    email VARCHAR(255) NOT NULL,
    website VARCHAR(255),
    company_name VARCHAR(255),

    -- Context
    reason_for_booking TEXT,
    how_did_you_hear VARCHAR(100),

    -- Current reality
    current_revenue VARCHAR(50),
    team_size VARCHAR(50),
    primary_service TEXT,
    average_deal_size VARCHAR(50),
    marketing_budget VARCHAR(50),
    biggest_bottleneck TEXT,
    is_decision_maker VARCHAR(20),
    previous_agency_experience TEXT,

    -- Process fields (for future use)
    acquisition_source TEXT,
    sales_process TEXT,
    fulfillment_workflow TEXT,
    current_tech_stack TEXT,

    -- Dream future
    desired_outcome TEXT,
    desired_speed VARCHAR(50),
    ready_to_scale VARCHAR(20),

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- AI Analysis results table
CREATE TABLE analysis_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id UUID NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,

    executive_summary TEXT,
    client_psychology TEXT,
    operational_gap_analysis TEXT,
    red_flags JSONB DEFAULT '[]',
    green_flags JSONB DEFAULT '[]',
    strategic_questions JSONB DEFAULT '[]',
    closing_strategy TEXT,
    estimated_fit_score INTEGER,

    raw_response JSONB, -- Store full Gemini response for debugging
    error_message TEXT, -- Store error if analysis failed

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_submissions_admin_token ON submissions(admin_token);
CREATE INDEX idx_submissions_status ON submissions(status);
CREATE INDEX idx_submissions_created_at ON submissions(created_at DESC);
CREATE INDEX idx_analysis_submission_id ON analysis_results(submission_id);
```

---

## API Endpoints

### POST `/api/submissions`
Create a new submission and trigger async AI analysis.

**Request Body:** JSON with all IntakeFormData fields (camelCase)

**Response:**
```json
{
  "id": "uuid",
  "adminUrl": "/admin/{adminToken}"
}
```

**Flow:**
1. Validate required fields (firstName, email)
2. Generate unique admin token (crypto-secure random 32-byte string, base64url encoded)
3. Insert submission with status "pending"
4. Spawn goroutine to call Gemini AI and store results
5. Return immediately with submission ID and admin URL

### GET `/api/admin/{adminToken}`
Retrieve submission and analysis results for admin view.

**Response:**
```json
{
  "submission": { /* IntakeFormData fields */ },
  "analysis": { /* AnalysisResult fields or null if pending */ },
  "status": "pending | processing | completed | failed",
  "createdAt": "2025-01-15T10:30:00Z"
}
```

### GET `/api/admin/{adminToken}/status`
Poll for analysis completion status (lightweight endpoint for polling).

**Response:**
```json
{
  "status": "pending | processing | completed | failed",
  "estimatedFitScore": 85
}
```

---

## Go Project Structure

```
backend/
├── cmd/
│   └── server/
│       └── main.go              # Entry point, router setup
├── internal/
│   ├── config/
│   │   └── config.go            # Environment config loading
│   ├── database/
│   │   ├── postgres.go          # DB connection setup
│   │   └── migrations/
│   │       └── 001_initial.sql  # Schema creation
│   ├── handlers/
│   │   ├── submissions.go       # POST /api/submissions
│   │   └── admin.go             # GET /api/admin/{token}
│   ├── models/
│   │   ├── submission.go        # Submission struct & DB methods
│   │   └── analysis.go          # AnalysisResult struct & DB methods
│   └── services/
│       └── gemini.go            # Gemini AI integration + async processing
├── pkg/
│   └── tokens/
│       └── generator.go         # Secure token generation
├── go.mod
├── go.sum
├── Dockerfile
└── docker-compose.yml           # Postgres + Go app for local dev
```

---

## Implementation Steps

### Phase 1: Project Setup
1. Create `backend/` directory
2. Initialize Go module: `go mod init github.com/tomr1233/booking-intake-form/backend`
3. Set up project directory structure
4. Add dependencies:
   ```bash
   go get github.com/gin-gonic/gin
   go get github.com/jackc/pgx/v5
   go get github.com/google/generative-ai-go
   go get github.com/joho/godotenv
   go get google.golang.org/api
   ```
5. Create docker-compose.yml for local Postgres:
   ```yaml
   services:
     postgres:
       image: postgres:16
       environment:
         POSTGRES_USER: clarity
         POSTGRES_PASSWORD: clarity
         POSTGRES_DB: clarity
       ports:
         - "5432:5432"
       volumes:
         - postgres_data:/var/lib/postgresql/data

   volumes:
     postgres_data:
   ```

### Phase 2: Database Layer
1. Write SQL migration file (`001_initial.sql`)
2. Implement database connection with pgx connection pool
3. Create Submission model with methods:
    - `Create(submission) -> (id, adminToken, error)`
    - `GetByAdminToken(token) -> (submission, error)`
    - `UpdateStatus(id, status) -> error`
4. Create AnalysisResult model with methods:
    - `Create(result) -> error`
    - `GetBySubmissionID(submissionID) -> (result, error)`

### Phase 3: API Handlers
1. Implement POST `/api/submissions` handler:
    - Parse JSON body into struct
    - Validate required fields
    - Call model to create submission
    - Spawn goroutine for async analysis
    - Return JSON response
2. Implement GET `/api/admin/{adminToken}` handler
3. Implement GET `/api/admin/{adminToken}/status` handler
4. Add CORS middleware (allow frontend origin)
5. Add request validation middleware

### Phase 4: Gemini Integration
1. Port `services/geminiService.ts` logic to Go
2. Use same prompt structure and JSON schema
3. Key prompt elements to preserve:
    - System instruction: "You are an expert sales analyst. Be direct, critical, and strategic."
    - Temperature: 0.3
    - Response schema matching AnalysisResult type
4. Implement async wrapper:
   ```go
   func (s *GeminiService) AnalyzeAsync(submissionID uuid.UUID, data IntakeFormData) {
       go func() {
           // Update status to "processing"
           s.db.UpdateSubmissionStatus(submissionID, "processing")

           // Call Gemini API
           result, err := s.Analyze(data)
           if err != nil {
               s.db.UpdateSubmissionStatus(submissionID, "failed")
               s.db.CreateAnalysisResult(submissionID, nil, err.Error())
               return
           }

           // Store result and update status
           s.db.CreateAnalysisResult(submissionID, result, "")
           s.db.UpdateSubmissionStatus(submissionID, "completed")
       }()
   }
   ```

### Phase 5: Frontend Updates
After backend is complete, update frontend:
1. Install react-router-dom: `npm install react-router-dom`
2. Create `services/api.ts` for backend calls
3. Create `components/ThankYouPage.tsx` component
4. Create `components/AdminResultsPage.tsx` (reuse AnalysisDashboard)
5. Update `App.tsx` with React Router
6. Update `components/IntakeWizard.tsx` to POST to backend and redirect
7. Delete or keep `services/geminiService.ts` (no longer used - AI runs on backend)

---

## Detailed Frontend Changes

### Files to Create

**1. `services/api.ts`** - Backend API client
```typescript
const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';

export interface SubmissionResponse {
  id: string;
  adminUrl: string;
}

export interface AdminResponse {
  submission: IntakeFormData;
  analysis: AnalysisResult | null;
  status: 'pending' | 'processing' | 'completed' | 'failed';
  createdAt: string;
}

export async function submitIntakeForm(data: IntakeFormData): Promise<SubmissionResponse> {
  const response = await fetch(`${API_URL}/api/submissions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!response.ok) throw new Error('Submission failed');
  return response.json();
}

export async function getAdminResults(token: string): Promise<AdminResponse> {
  const response = await fetch(`${API_URL}/api/admin/${token}`);
  if (!response.ok) throw new Error('Failed to fetch results');
  return response.json();
}
```

**2. `components/ThankYouPage.tsx`** - Shown after submission
```typescript
import React from 'react';

export const ThankYouPage: React.FC = () => {
  return (
    <div className="min-h-[calc(100vh-64px)] flex items-center justify-center py-12">
      <div className="max-w-2xl mx-auto px-4 text-center">
        <div className="bg-white rounded-2xl shadow-xl p-10">
          <div className="w-16 h-16 bg-green-100 rounded-full flex items-center justify-center mx-auto mb-6">
            <svg className="w-8 h-8 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
          </div>
          <h1 className="text-3xl font-bold text-slate-800 mb-4">Thank You!</h1>
          <p className="text-slate-600 mb-2">Your application has been submitted successfully.</p>
          <p className="text-slate-500 text-sm">We'll review your information and see you on the call!</p>
        </div>
      </div>
    </div>
  );
};
```

**3. `components/AdminResultsPage.tsx`** - Admin view with polling
```typescript
import React, { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { getAdminResults, AdminResponse } from '../services/api';
import { AnalysisDashboard } from './AnalysisDashboard';

export const AdminResultsPage: React.FC = () => {
  const { token } = useParams<{ token: string }>();
  const [data, setData] = useState<AdminResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!token) return;

    const fetchResults = async () => {
      try {
        const result = await getAdminResults(token);
        setData(result);

        // Keep polling if still processing
        if (result.status === 'pending' || result.status === 'processing') {
          setTimeout(fetchResults, 3000); // Poll every 3 seconds
        }
      } catch (e) {
        setError('Failed to load results');
      }
    };

    fetchResults();
  }, [token]);

  if (error) return <div className="p-8 text-red-600">{error}</div>;
  if (!data) return <div className="p-8">Loading...</div>;

  if (data.status === 'pending' || data.status === 'processing') {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin w-8 h-8 border-4 border-brand-600 border-t-transparent rounded-full mx-auto mb-4"></div>
          <p className="text-slate-600">Analyzing submission...</p>
        </div>
      </div>
    );
  }

  if (data.status === 'failed' || !data.analysis) {
    return <div className="p-8 text-red-600">Analysis failed. Please try again.</div>;
  }

  return (
    <div className="py-12 px-4 sm:px-6">
      <AnalysisDashboard
        data={data.submission}
        analysis={data.analysis}
        onReset={() => {}} // No reset for admin view
      />
    </div>
  );
};
```

### Files to Modify

**1. `App.tsx`** - Add React Router
```typescript
import React from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { IntakeWizard } from './components/IntakeWizard';
import { ThankYouPage } from './components/ThankYouPage';
import { AdminResultsPage } from './components/AdminResultsPage';

const App: React.FC = () => {
  return (
    <BrowserRouter>
      <div className="min-h-screen bg-slate-50 font-sans text-slate-900">
        <nav className="bg-white border-b border-slate-200 sticky top-0 z-50">
          {/* ... existing nav code ... */}
        </nav>

        <Routes>
          <Route path="/" element={<IntakeWizard />} />
          <Route path="/thank-you" element={<ThankYouPage />} />
          <Route path="/admin/:token" element={<AdminResultsPage />} />
        </Routes>

        {/* ... existing styles ... */}
      </div>
    </BrowserRouter>
  );
};

export default App;
```

**2. `components/IntakeWizard.tsx`** - POST to backend, redirect
```typescript
// Remove this import:
// import { analyzeIntakeForm } from '../services/geminiService';

// Add these imports:
import { useNavigate } from 'react-router-dom';
import { submitIntakeForm } from '../services/api';

// Remove the prop interface and onAnalysisComplete prop:
// interface IntakeWizardProps {
//   onAnalysisComplete: (data: IntakeFormData, analysis: AnalysisResult) => void;
// }

export const IntakeWizard: React.FC = () => {
  const navigate = useNavigate();
  // ... existing state ...

  const handleSubmit = async () => {
    setIsAnalyzing(true);
    try {
      await submitIntakeForm(formData);
      navigate('/thank-you');
    } catch (e) {
      console.error(e);
      alert("Something went wrong. Please try again.");
    } finally {
      setIsAnalyzing(false);
    }
  };

  // ... rest of component unchanged ...
};
```

### Files to Delete (optional)
- `services/geminiService.ts` - No longer needed, AI runs on backend

### Environment Variables
Add to `.env.local`:
```
VITE_API_URL=http://localhost:8080
```

---

## Environment Variables

```env
# Database
DATABASE_URL=postgres://clarity:clarity@localhost:5432/clarity

# Gemini AI
GEMINI_API_KEY=your-api-key

# Server
PORT=8080
FRONTEND_URL=http://localhost:3000

# Security
ADMIN_TOKEN_LENGTH=32
```

---

## Key Files to Create

| File | Purpose |
|------|---------|
| `backend/cmd/server/main.go` | Server entry point, router setup, middleware |
| `backend/internal/config/config.go` | Load and validate env vars |
| `backend/internal/database/postgres.go` | DB connection pool with pgx |
| `backend/internal/database/migrations/001_initial.sql` | Schema creation SQL |
| `backend/internal/models/submission.go` | Submission struct, Create/GetByToken/UpdateStatus |
| `backend/internal/models/analysis.go` | AnalysisResult struct, Create/GetBySubmissionID |
| `backend/internal/handlers/submissions.go` | POST /api/submissions handler |
| `backend/internal/handlers/admin.go` | GET /api/admin/{token} handlers |
| `backend/internal/services/gemini.go` | Gemini AI client + AnalyzeAsync |
| `backend/pkg/tokens/generator.go` | Secure admin token generation |
| `backend/docker-compose.yml` | Local Postgres for development |
| `backend/Dockerfile` | Production container build |

---

## Gemini Prompt (port from geminiService.ts)

```go
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

Analyze this prospect and provide:
1. Executive summary (2 sentences)
2. Client psychology based on their writing style
3. Operational gap analysis
4. Red flags (disqualifying risks)
5. Green flags (positive indicators)
6. 3-5 strategic questions for the sales call
7. Closing strategy based on their desired outcome
8. Fit score (0-100)`
```

---

## Security Considerations

1. **Admin tokens**: Use `crypto/rand` for secure 32-byte tokens, encode as base64url
2. **Rate limiting**: Add rate limiting on submission endpoint (consider golang.org/x/time/rate)
3. **Input validation**: Validate email format, limit text field lengths
4. **CORS**: Restrict to FRONTEND_URL origin only
5. **SQL injection**: Use parameterized queries (pgx handles this automatically)
6. **No auth on admin URLs**: Long random tokens ARE the authentication

---

## Testing Strategy

1. **Unit tests**:
    - Token generation produces valid base64url
    - Model methods work correctly
    - Gemini prompt formatting
2. **Integration tests**:
    - Full API flow with test database
    - Submission creates pending status
    - Status updates after analysis
3. **Manual testing**:
    - Submit form, verify thank you page
    - Check admin URL shows results after processing

---

## Deployment Notes

- Set DATABASE_URL to production Postgres connection string
- Store GEMINI_API_KEY in secrets manager (not env file)
- Run migrations on deploy: `psql $DATABASE_URL -f migrations/001_initial.sql`
- Add health check endpoint: `GET /health` returning `{"status": "ok"}`
- Consider adding structured logging (zerolog or zap)
