package clientapi

type StartRequest struct {
	RunID        string `json:"run_id,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	UserPrompt   string `json:"user_prompt"`
	MaxSteps     *int   `json:"max_steps,omitempty"`
}

type ContinueRequest struct {
	MaxSteps   *int        `json:"max_steps,omitempty"`
	Resolution *Resolution `json:"resolution,omitempty"`
}

type Resolution struct {
	RequirementID string `json:"requirement_id"`
	Kind          string `json:"kind"`
	Outcome       string `json:"outcome"`
	Value         string `json:"value,omitempty"`
}

type SteerRequest struct {
	Instruction string `json:"instruction"`
}

type FollowUpRequest struct {
	Prompt   string `json:"prompt"`
	MaxSteps *int   `json:"max_steps,omitempty"`
}

type RunState struct {
	RunID              string              `json:"run_id"`
	Status             string              `json:"status"`
	Step               int                 `json:"step"`
	Version            int64               `json:"version"`
	Output             string              `json:"output,omitempty"`
	Error              string              `json:"error,omitempty"`
	PendingRequirement *PendingRequirement `json:"pending_requirement,omitempty"`
}

type PendingRequirement struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Prompt string `json:"prompt,omitempty"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
