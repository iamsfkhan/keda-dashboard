package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/iamsfkhan/keda-dashboard/internal/model"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 8 * time.Second}}
}

func (c *Client) Enabled() bool { return c != nil && c.baseURL != "" }

func (c *Client) History(ctx context.Context, namespace, name, kind string) ([]model.MetricPoint, error) {
	if !c.Enabled() {
		return []model.MetricPoint{}, nil
	}
	query := fmt.Sprintf(`sum(keda_scaler_metrics_value{namespace=%q,scaledObject=%q})`, namespace, name)
	if kind == "scaledjobs" {
		query = fmt.Sprintf(`sum(keda_scaler_metrics_value{namespace=%q,scaledJob=%q})`, namespace, name)
	}
	now := time.Now()
	values := url.Values{
		"query": {query},
		"start": {strconv.FormatInt(now.Add(-time.Hour).Unix(), 10)},
		"end":   {strconv.FormatInt(now.Unix(), 10)},
		"step":  {"60"},
	}
	endpoint := c.baseURL + "/api/v1/query_range?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query prometheus: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned %s", resp.Status)
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Values [][]any `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode prometheus response: %w", err)
	}
	points := []model.MetricPoint{}
	if len(payload.Data.Result) == 0 {
		return points, nil
	}
	for _, pair := range payload.Data.Result[0].Values {
		if len(pair) != 2 {
			continue
		}
		ts, ok := pair[0].(float64)
		value, err := strconv.ParseFloat(fmt.Sprint(pair[1]), 64)
		if !ok || err != nil {
			continue
		}
		points = append(points, model.MetricPoint{Timestamp: int64(ts), Value: value})
	}
	return points, nil
}
