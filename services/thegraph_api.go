package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// TheGraphAPI handles pool queries using The Graph API
type TheGraphAPI struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewTheGraphAPI creates a new The Graph API service
func NewTheGraphAPI() *TheGraphAPI {
	token := os.Getenv("THEGRAPH_API_TOKEN")
	if token == "" {
		// Use default token if not set in environment
		token = "eyJhbGciOiJLTVNFUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE4MDE1Mzg3OTMsImp0aSI6ImRmYTk0MzU1LWRmY2YtNGJiYS04N2ExLTM2ODdkOWZhMGVjNiIsImlhdCI6MTc2NTUzODc5MywiaXNzIjoiZGZ1c2UuaW8iLCJzdWIiOiIwc3VyYWQ3YTAwNTcwNjQyY2M0ODQiLCJ2IjoyLCJha2kiOiIzZTdkNjI1ODAzMjY0YjE2NTBkZjI5MWZlNWM1NzUzZTg4NjhhOWYxNTEzMTQ5MzkxODJiZTE0OWYwNzg1NDcyIiwidWlkIjoiMHN1cmFkN2EwMDU3MDY0MmNjNDg0Iiwic3Vic3RyZWFtc19wbGFuX3RpZXIiOiJGUkVFIiwiY2ZnIjp7IlNVQlNUUkVBTVNfTUFYX1JFUVVFU1RTIjoiMiIsIlNVQlNUUkVBTVNfUEFSQUxMRUxfSk9CUyI6IjUiLCJTVUJTVFJFQU1TX1BBUkFMTEVMX1dPUktFUlMiOiI1In0sInRva2VuX2FwaV9wbGFuX3RpZXIiOiJGUkVFIiwidG9rZW5fYXBpX2ZlYXR1cmVfY29uZmlncyI6eyJUT0tFTl9BUElfQkFUQ0hfU0laRSI6IjEiLCJUT0tFTl9BUElfSVRFTVNfUkVUVVJORUQiOiIxMCIsIlRPS0VOX0FQSV9NQVhJTVVNX0FMTE9XRURfRU5EUE9JTlRfR1JPVVAiOiJuZnQiLCJUT0tFTl9BUElfUExBTl9DUkVESVRTX0NFTlRTIjoiMjUwMCIsIlRPS0VOX0FQSV9SQVRFX0xJTUlUX1BFUl9NSU5VVEUiOiIyMDAiLCJUT0tFTl9BUElfUkVBTF9USU1FX0RBVEEiOiJ0cnVlIn19.VT3TdNkS8cPKR3ElUBs7OBSblVNbYYv063ZrJ7vh24-v383UE3JA7zkGr7SG23d1nerlnEU-CJ5_HxMF0dGKkA"
	}

	return &TheGraphAPI{
		baseURL: "https://token-api.thegraph.com/v1/evm/pools",
		token:   token,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// TokenInfo represents token information from The Graph API
type TokenInfo struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

// PoolData represents pool data from The Graph API
type PoolData struct {
	Factory     string    `json:"factory"`
	Pool        string    `json:"pool"`
	InputToken  TokenInfo `json:"input_token"`
	OutputToken TokenInfo `json:"output_token"`
	Fee         int       `json:"fee"`
	Protocol    string    `json:"protocol"`
	Network     string    `json:"network"`
}

// TheGraphResponse represents the API response structure
type TheGraphResponse struct {
	Data    []PoolData `json:"data"`
	Results int        `json:"results"`
}

// QueryPool queries pool information from The Graph API
func (api *TheGraphAPI) QueryPool(network, poolID string) (*PoolData, error) {
	// Build URL with parameters
	params := url.Values{}
	params.Add("network", network)
	params.Add("pool", poolID)
	params.Add("limit", "10")
	params.Add("page", "1")

	requestURL := fmt.Sprintf("%s?%s", api.baseURL, params.Encode())

	// Create request
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// Add authorization header
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", api.token))
	req.Header.Add("Accept", "application/json")

	// Execute request
	resp, err := api.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("API 速率限制，请稍后重试")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回错误 (状态码 %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var response TheGraphResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// Check if we got results
	if response.Results == 0 || len(response.Data) == 0 {
		return nil, fmt.Errorf("未找到池子信息，请检查池子地址或 PoolId 是否正确")
	}

	return &response.Data[0], nil
}
