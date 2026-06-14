package controlplane

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"go.etcd.io/etcd/client/v3"
)

// Handler serves HTTP endpoints for administrative CRUD operations on policies in etcd.
type Handler struct {
	client *Client
}

// NewHandler creates a new control plane HTTP handler.
func NewHandler(client *Client) *Handler {
	return &Handler{client: client}
}

// ServeHTTP routes administrative requests to appropriate handler operations.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		h.listPolicies(w, r)
	case http.MethodPost:
		h.savePolicy(w, r)
	case http.MethodDelete:
		h.deletePolicy(w, r)
	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method_not_allowed"})
	}
}

// listPolicies retrieves all policies stored in etcd.
func (h *Handler) listPolicies(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	cli := h.client.GetEtcdClient()
	prefix := h.client.Prefix()

	resp, err := cli.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		slog.Error("failed to query policies from etcd", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed_to_query_policies"})
		return
	}

	policies := make([]Policy, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var p Policy
		if err := json.Unmarshal(kv.Value, &p); err == nil {
			policies = append(policies, p)
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(policies)
}

// savePolicy parses a policy configuration and saves it to etcd.
func (h *Handler) savePolicy(w http.ResponseWriter, r *http.Request) {
	var p Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_policy_payload"})
		return
	}

	// Basic Validation
	if p.ID == "" || p.Path == "" || p.Limit <= 0 || p.WindowSec <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing_required_fields"})
		return
	}

	if p.Method == "" {
		p.Method = "*"
	}
	if p.Tier == "" {
		p.Tier = "*"
	}

	payload, err := json.Marshal(p)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed_to_serialize_policy"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	cli := h.client.GetEtcdClient()
	key := h.client.Prefix() + p.ID

	_, err = cli.Put(ctx, key, string(payload))
	if err != nil {
		slog.Error("failed to write policy to etcd", "id", p.ID, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed_to_write_policy"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "saved", "id": p.ID})
}

// deletePolicy removes a policy from etcd using its ID.
func (h *Handler) deletePolicy(w http.ResponseWriter, r *http.Request) {
	policyID := r.URL.Query().Get("id")
	if policyID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing_policy_id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	cli := h.client.GetEtcdClient()
	key := h.client.Prefix() + policyID

	// Delete from etcd
	resp, err := cli.Delete(ctx, key)
	if err != nil {
		slog.Error("failed to delete policy from etcd", "id", policyID, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed_to_delete_policy"})
		return
	}

	if resp.Deleted == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "policy_not_found"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "id": policyID})
}
