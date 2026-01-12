package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"jobplane/pkg/api"
	"net/http"
	"time"
)

// JobClient handles API calls to the jobplane controller.
type JobClient struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewJobClient creates a new client with the given base URL and token.
func NewJobClient(baseURL, token string) *JobClient {
	return &JobClient{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// APIError represents an error response from the API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error (%d): %s", e.StatusCode, e.Message)
}

// CreateJob sends POST /jobs to create a new job definition.
func (c *JobClient) CreateJob(req api.CreateJobRequest) (*api.CreateJobResponse, error) {
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/jobs", c.BaseURL), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	httpReq.Header.Add("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	var result api.CreateJobResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// RunJob sends POST /jobs/{id}/run to trigger a new execution.
func (c *JobClient) RunJob(jobID string, req api.RunJobRequest) (*api.RunJobResponse, error) {
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("invalid request body")
	}

	httpReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/jobs/%s/run", c.BaseURL, jobID), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	httpReq.Header.Add("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	var result api.RunJobResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// GetExecution sends GET /executions/{id} to retrieve execution details.
func (c *JobClient) GetExecution(executionID string) (*api.ExecutionResponse, error) {
	httpReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/executions/%s", c.BaseURL, executionID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	httpReq.Header.Add("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	var result api.ExecutionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// GetLogs sends GET /executions/{id}/logs to retrieve execution logs.
func (c *JobClient) GetLogs(executionID string, afterID int64) ([]api.LogEntry, error) {
	endpoint := fmt.Sprintf("%s/executions/%s/logs?after_id=%d", c.BaseURL, executionID, afterID)
	httpReq, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	httpReq.Header.Add("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	var result api.GetLogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Logs, nil
}

// ListDLQExecutions sends GET /executions/dlq to retrieve failed executions.
func (c *JobClient) ListDLQExecutions(limit, offset int) ([]api.DLQExecutionResponse, error) {
	endpoint := fmt.Sprintf("%s/executions/dlq?limit=%d&offset=%d", c.BaseURL, limit, offset)
	httpReq, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	httpReq.Header.Add("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	var result []api.DLQExecutionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// RetryDLQExecution sends POST /executions/dlq/{id}/retry to retry a failed execution.
func (c *JobClient) RetryDLQExecution(executionID string) (*api.RetryDQLExecutionResponse, error) {
	endpoint := fmt.Sprintf("%s/executions/dlq/%s/retry", c.BaseURL, executionID)
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	httpReq.Header.Add("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	var result api.RetryDQLExecutionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}
