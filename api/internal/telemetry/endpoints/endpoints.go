package endpoints

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ElasticClient is a minimal Elasticsearch client for read-only queries.
type ElasticClient struct {
	http  *http.Client
	url   string
	token string
}

func NewElasticClient(url, username, password string) *ElasticClient {
	return &ElasticClient{
		http: &http.Client{Timeout: 10 * time.Second},
		url:  strings.TrimRight(url, "/"),
		token: "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password)),
	}
}

func (c *ElasticClient) search(index string, query any) (map[string]any, error) {
	body := map[string]any{"query": query, "size": 100}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, c.url+"/"+index+"/_search", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// LogsByCase handles GET /internal/v1/telemetry/logs?case_id=&window=.
// Read-only agent tool: searches Elastic logs scoped to a case.
func LogsByCase(w http.ResponseWriter, r *http.Request) {
	caseID := r.URL.Query().Get("case_id")
	window := r.URL.Query().Get("window")
	if caseID == "" {
		http.Error(w, `{"error":"case_id is required"}`, http.StatusBadRequest)
		return
	}

	client := getElasticClient()
	query := map[string]any{
		"bool": map[string]any{
			"filter": []map[string]any{
				{"term": map[string]any{"case_id": caseID}},
			},
		},
	}
	if window != "" {
		query["bool"].(map[string]any)["filter"] = append(query["bool"].(map[string]any)["filter"].([]map[string]any), map[string]any{
			"range": map[string]any{
				"@timestamp": map[string]any{"gte": "now-" + window},
			},
		})
	}

	result, err := client.search("fractal-logs", query)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"elasticsearch query failed: `+err.Error()+`"}`, http.StatusBadGateway)
		return
	}

	hits := []map[string]any{}
	if hitsInterface, ok := result["hits"].(map[string]any); ok {
		if hitsList, ok := hitsInterface["hits"].([]any); ok {
			for _, h := range hitsList {
				if hit, ok := h.(map[string]any); ok {
					if source, ok := hit["_source"].(map[string]any); ok {
						hits = append(hits, source)
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"case_id": caseID,
		"window":  window,
		"hits":    hits,
	})
}

// TraceByID handles GET /internal/v1/telemetry/traces/{trace_id}.
// Read-only agent tool: fetches a single trace span tree.
func TraceByID(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("trace_id")
	if traceID == "" {
		http.Error(w, `{"error":"trace_id is required"}`, http.StatusBadRequest)
		return
	}

	client := getElasticClient()
	query := map[string]any{
		"bool": map[string]any{
			"filter": []map[string]any{
				{"term": map[string]any{"trace_id": traceID}},
			},
		},
	}

	result, err := client.search("fractal-traces", query)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"elasticsearch query failed: `+err.Error()+`"}`, http.StatusBadGateway)
		return
	}

	spans := []map[string]any{}
	if hitsInterface, ok := result["hits"].(map[string]any); ok {
		if hitsList, ok := hitsInterface["hits"].([]any); ok {
			for _, h := range hitsList {
				if hit, ok := h.(map[string]any); ok {
					if source, ok := hit["_source"].(map[string]any); ok {
						spans = append(spans, source)
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"trace_id": traceID,
		"spans":    spans,
	})
}

// HandlerErrorRate handles GET /internal/v1/telemetry/handler-error-rate?merchant_id=&window=.
// Read-only agent tool: aggregate metric for per-merchant handler failure rate.
func HandlerErrorRate(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")
	window := r.URL.Query().Get("window")
	if merchantID == "" {
		http.Error(w, `{"error":"merchant_id is required"}`, http.StatusBadRequest)
		return
	}

	client := getElasticClient()
	query := map[string]any{
		"bool": map[string]any{
			"filter": []map[string]any{
				{"term": map[string]any{"merchant_id": merchantID}},
				{"term": map[string]any{"event_type": "webhook_handler_failure"}},
			},
		},
	}
	if window != "" {
		query["bool"].(map[string]any)["filter"] = append(query["bool"].(map[string]any)["filter"].([]map[string]any), map[string]any{
			"range": map[string]any{
				"@timestamp": map[string]any{"gte": "now-" + window},
			},
		})
	}

	result, err := client.search("fractal-metrics", query)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"elasticsearch query failed: `+err.Error()+`"}`, http.StatusBadGateway)
		return
	}

	count := 0
	if hitsInterface, ok := result["hits"].(map[string]any); ok {
		if total, ok := hitsInterface["total"].(map[string]any); ok {
			if v, ok := total["value"].(float64); ok {
				count = int(v)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"merchant_id": merchantID,
		"window":      window,
		"error_rate":  0.0,
		"count":       count,
	})
}

var elasticClient *ElasticClient

func getElasticClient() *ElasticClient {
	if elasticClient != nil {
		return elasticClient
	}
	url := os.Getenv("ELASTICSEARCH_URL")
	user := os.Getenv("ELASTIC_USERNAME")
	pass := os.Getenv("ELASTIC_PASSWORD")
	if url == "" {
		url = "http://localhost:9200"
	}
	elasticClient = NewElasticClient(url, user, pass)
	return elasticClient
}
