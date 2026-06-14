package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.etcd.io/etcd/client/v3"
)

// Policy defines the rate limiting or routing configuration for a resource path/tier.
type Policy struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Method    string `json:"method"` // GET, POST, or "*" for any
	Limit     int    `json:"limit"`
	WindowSec int    `json:"window_sec"`
	Tier      string `json:"tier"` // basic, premium, or "*" for any
}

// Client manages connection to the etcd cluster and hot reloads policies.
type Client struct {
	cli              *clientv3.Client
	policies         sync.Map // key: policyID -> Policy
	prefix           string
	shutdownCh       chan struct{}
	wg               sync.WaitGroup
	onUpstreamUpdate func(string)
}

// NewClient creates a new etcd configuration client.
func NewClient(endpoints []string, timeout time.Duration) (*Client, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	c := &Client{
		cli:        cli,
		prefix:     "/zengate/policies/",
		shutdownCh: make(chan struct{}),
	}

	return c, nil
}

// Start loads initial policies and starts the background etcd watch loop.
func (c *Client) Start(ctx context.Context) error {
	if c.cli == nil {
		slog.Warn("etcd client is nil, dynamic configuration watcher is disabled (using in-memory mode)")
		return nil
	}
	slog.Info("loading initial configuration from etcd", "prefix", c.prefix)

	// Fetch all existing policies
	resp, err := c.cli.Get(ctx, c.prefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to load initial policies: %w", err)
	}

	for _, kv := range resp.Kvs {
		c.updateLocalCache(kv.Key, kv.Value)
	}

	slog.Info("initial policy configuration loaded", "count", len(resp.Kvs))

	// Start watch loop
	c.wg.Add(1)
	go c.watchLoop()

	return nil
}

// Close gracefully stops the watch loop and closes the etcd connection.
func (c *Client) Close() error {
	close(c.shutdownCh)
	c.wg.Wait()
	if c.cli == nil {
		return nil
	}
	return c.cli.Close()
}

// GetPolicy retrieves a policy from local memory cache by ID.
func (c *Client) GetPolicy(id string) (Policy, bool) {
	val, ok := c.policies.Load(id)
	if !ok {
		return Policy{}, false
	}
	return val.(Policy), true
}

// AddPolicyToCache adds or updates a policy in the local memory cache.
func (c *Client) AddPolicyToCache(p Policy) {
	c.policies.Store(p.ID, p)
	slog.Info("policy cache updated manually (cache-only)", "id", p.ID, "path", p.Path, "limit", p.Limit)
}

// GetAllPolicies returns a slice of all policies stored in the local memory cache.
func (c *Client) GetAllPolicies() []Policy {
	policies := make([]Policy, 0)
	c.policies.Range(func(key, value interface{}) bool {
		policies = append(policies, value.(Policy))
		return true
	})
	return policies
}

// SetUpstreamUpdateCallback registers a callback to invoke when upstream is updated (used for cache-only/mock testing).
func (c *Client) SetUpstreamUpdateCallback(cb func(string)) {
	c.onUpstreamUpdate = cb
}

// UpdateUpstream triggers the local dynamic upstream target updater callback.
func (c *Client) UpdateUpstream(newTarget string) {
	if c.onUpstreamUpdate != nil {
		c.onUpstreamUpdate(newTarget)
	}
}

// GetMatchingPolicy finds a policy matching the incoming request parameters (path, method, tier).
func (c *Client) GetMatchingPolicy(path, method, tier string) (Policy, bool) {
	var matched Policy
	found := false

	c.policies.Range(func(key, value interface{}) bool {
		p := value.(Policy)
		
		// Match path (exact match for now, or match prefix/wildcard)
		pathMatch := p.Path == "*" || p.Path == path
		
		// Match method
		methodMatch := p.Method == "*" || p.Method == method
		
		// Match tier
		tierMatch := p.Tier == "*" || p.Tier == tier

		if pathMatch && methodMatch && tierMatch {
			matched = p
			found = true
			return false // stop iteration
		}
		return true // continue iteration
	})

	return matched, found
}

// GetEtcdClient returns the underlying raw etcd client (for control plane API writes).
func (c *Client) GetEtcdClient() *clientv3.Client {
	return c.cli
}

// Prefix returns the policy key prefix.
func (c *Client) Prefix() string {
	return c.prefix
}

// --- Internal helpers ---

func (c *Client) watchLoop() {
	defer c.wg.Done()

	if c.cli == nil {
		return
	}

	watchChan := c.cli.Watch(context.Background(), c.prefix, clientv3.WithPrefix())
	slog.Info("started configuration watcher loop", "prefix", c.prefix)

	for {
		select {
		case <-c.shutdownCh:
			slog.Info("stopping configuration watcher loop")
			return
		case wresp, ok := <-watchChan:
			if !ok {
				slog.Warn("etcd watch channel closed, restarting watch in 2 seconds...")
				time.Sleep(2 * time.Second)
				watchChan = c.cli.Watch(context.Background(), c.prefix, clientv3.WithPrefix())
				continue
			}

			for _, ev := range wresp.Events {
				switch ev.Type {
				case clientv3.EventTypePut:
					c.updateLocalCache(ev.Kv.Key, ev.Kv.Value)
				case clientv3.EventTypeDelete:
					c.deleteFromLocalCache(ev.Kv.Key)
				}
			}
		}
	}
}

func (c *Client) updateLocalCache(key, value []byte) {
	var p Policy
	if err := json.Unmarshal(value, &p); err != nil {
		slog.Error("failed to decode policy JSON from etcd", "key", string(key), "error", err)
		return
	}

	c.policies.Store(p.ID, p)
	slog.Info("policy cache hot-reloaded (PUT)", "id", p.ID, "path", p.Path, "limit", p.Limit, "window", p.WindowSec)
}

func (c *Client) deleteFromLocalCache(key []byte) {
	// Key is "/zengate/policies/<policy_id>"
	// We extract the policy_id suffix
	policyID := string(key[len(c.prefix):])
	c.policies.Delete(policyID)
	slog.Info("policy cache hot-reloaded (DELETE)", "id", policyID)
}
